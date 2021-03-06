/*
Copyright (c) 2019 VMware, Inc. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vmware/octant/internal/log"

	// auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

//go:generate mockgen -source=cluster.go -destination=./fake/mock_client_interface.go -package=fake github.com/vmware/octant/internal/cluster ClientInterface
//go:generate mockgen -source=../../vendor/k8s.io/client-go/dynamic/dynamicinformer/interface.go -destination=./fake/mock_dynamicinformer.go -package=fake k8s.io/client-go/dynamic/dynamicinformer DynamicSharedInformerFactory
//go:generate mockgen -source=../../vendor/k8s.io/client-go/informers/generic.go -destination=./fake/mock_genericinformer.go -package=fake k8s.io/client-go/informers GenericInformer
//go:generate mockgen -source=../../vendor/k8s.io/client-go/discovery/discovery_client.go -imports=openapi_v2=github.com/googleapis/gnostic/OpenAPIv2 -destination=./fake/mock_discoveryinterface.go -package=fake k8s.io/client-go/discovery DiscoveryInterface
//go:generate mockgen -source=../../vendor/k8s.io/client-go/kubernetes/clientset.go -destination=./fake/mock_kubernetes_client.go -package=fake -mock_names=Interface=MockKubernetesInterface k8s.io/client-go/kubernetes Interface
//go:generate mockgen -destination=./fake/mock_sharedindexinformer.go -package=fake k8s.io/client-go/tools/cache SharedIndexInformer
//go:generate mockgen -destination=./fake/mock_authorization.go -package=fake k8s.io/client-go/kubernetes/typed/authorization/v1 AuthorizationV1Interface,SelfSubjectAccessReviewInterface,SelfSubjectAccessReviewsGetter,SelfSubjectRulesReviewInterface,SelfSubjectRulesReviewsGetter
//go:generate mockgen -source=../../vendor/k8s.io/client-go/dynamic/interface.go -destination=./fake/mock_dynamic_client.go -package=fake -imports=github.com/vmware/octant/vendor/k8s.io/client-go/dynamic=k8s.io/client-go/dynamic -mock_names=Interface=MockDynamicInterface k8s.io/client-go/dynamic Interface

// ClientInterface is a client for cluster operations.
type ClientInterface interface {
	ResourceExists(schema.GroupVersionResource) bool
	Resource(schema.GroupKind) (schema.GroupVersionResource, error)
	KubernetesClient() (kubernetes.Interface, error)
	DynamicClient() (dynamic.Interface, error)
	DiscoveryClient() (discovery.DiscoveryInterface, error)
	NamespaceClient() (NamespaceInterface, error)
	InfoClient() (InfoInterface, error)
	Close()
	RESTInterface
}

type RESTInterface interface {
	RESTClient() (rest.Interface, error)
	RESTConfig() *rest.Config
}

// Cluster is a client for cluster operations
type Cluster struct {
	clientConfig clientcmd.ClientConfig
	restConfig   *rest.Config
	logger       log.Logger

	kubernetesClient kubernetes.Interface
	dynamicClient    dynamic.Interface
	discoveryClient  discovery.DiscoveryInterface

	restMapper *restmapper.DeferredDiscoveryRESTMapper

	closeFn context.CancelFunc
}

var _ ClientInterface = (*Cluster)(nil)

func newCluster(ctx context.Context, clientConfig clientcmd.ClientConfig, restClient *rest.Config) (*Cluster, error) {
	logger := log.From(ctx).With("component", "cluster client")

	kubernetesClient, err := kubernetes.NewForConfig(restClient)
	if err != nil {
		return nil, errors.Wrap(err, "create kubernetes client")
	}

	dynamicClient, err := dynamic.NewForConfig(restClient)
	if err != nil {
		return nil, errors.Wrap(err, "create dynamic client")
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restClient)
	if err != nil {
		return nil, errors.Wrap(err, "create discovery client")
	}

	dir, err := ioutil.TempDir("", "octant")
	if err != nil {
		return nil, errors.Wrap(err, "create temp directory")
	}

	logger.With("dir", dir).Debugf("created temp directory")

	cachedDiscoveryClient, err := discovery.NewCachedDiscoveryClientForConfig(
		restClient,
		dir,
		dir,
		180*time.Second,
	)
	if err != nil {
		return nil, errors.Wrap(err, "create cached discovery client")
	}

	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDiscoveryClient)

	c := &Cluster{
		clientConfig:     clientConfig,
		restConfig:       restClient,
		kubernetesClient: kubernetesClient,
		dynamicClient:    dynamicClient,
		discoveryClient:  discoveryClient,
		restMapper:       restMapper,
		logger:           log.From(ctx),
	}

	ctx, cancel := context.WithCancel(ctx)
	c.closeFn = cancel

	go func() {
		<-ctx.Done()
		logger.Infof("removing cluster client temporary directory")

		if err := os.RemoveAll(dir); err != nil {
			logger.WithErr(err).Errorf("closing temporary directory")
		}
	}()

	return c, nil
}

func (c *Cluster) Close() {
	if c.closeFn != nil {
		c.closeFn()
	}
}

func (c *Cluster) ResourceExists(gvr schema.GroupVersionResource) bool {
	restMapper := c.restMapper
	_, err := restMapper.KindFor(gvr)
	return err == nil
}

func (c *Cluster) Resource(gk schema.GroupKind) (schema.GroupVersionResource, error) {
	restConfig, err := c.clientConfig.ClientConfig()
	if err != nil {
		return schema.GroupVersionResource{}, errors.Wrap(err, "get rest config")
	}

	retries := 0
	resetRestMapper := false

	for retries < 5 {
		restMapping, err := c.restMapper.RESTMapping(gk)
		if err != nil {
			if meta.IsNoMatchError(err) {
				if !resetRestMapper {
					c.restMapper.Reset()
					resetRestMapper = true
					continue
				}

				retries++
				c.logger.
					WithErr(err).
					With("group-kind", gk.String()).
					Errorf("Having trouble retrieving the REST mapping from your cluster at %s. Retrying.....", restConfig.Host)
				time.Sleep(5 * time.Second)

				continue
			}
			return schema.GroupVersionResource{}, errors.Wrap(err, "unable to retrieve rest mapping")
		}
		return restMapping.Resource, nil
	}
	c.logger.
		With("group-kind", gk.String()).
		Errorf("Unable to retrieve the REST mapping from your cluster at %s. Full error details below.", restConfig.Host)
	return schema.GroupVersionResource{}, errors.New("unable to retrieve rest mapping")
}

// KubernetesClient returns a Kubernetes client.
func (c *Cluster) KubernetesClient() (kubernetes.Interface, error) {
	return c.kubernetesClient, nil
}

// NamespaceClient returns a namespace client.
func (c *Cluster) NamespaceClient() (NamespaceInterface, error) {
	dc, err := c.DynamicClient()
	if err != nil {
		return nil, err
	}

	ns, _, err := c.clientConfig.Namespace()
	if err != nil {
		return nil, errors.Wrap(err, "resolving initial namespace")
	}
	return newNamespaceClient(dc, ns), nil
}

// DynamicClient returns a dynamic client.
func (c *Cluster) DynamicClient() (dynamic.Interface, error) {
	return c.dynamicClient, nil
}

// DiscoveryClient returns a DiscoveryClient for the cluster.
func (c *Cluster) DiscoveryClient() (discovery.DiscoveryInterface, error) {
	return c.discoveryClient, nil
}

// InfoClient returns an InfoClient for the cluster.
func (c *Cluster) InfoClient() (InfoInterface, error) {
	return newClusterInfo(c.clientConfig), nil
}

// RESTClient returns a RESTClient for the cluster.
func (c *Cluster) RESTClient() (rest.Interface, error) {
	config := withConfigDefaults(c.restConfig)
	return rest.RESTClientFor(config)
}

// RESTConfig returns configuration for communicating with the cluster.
func (c *Cluster) RESTConfig() *rest.Config {
	return c.restConfig
}

// Version returns a ServerVersion for the cluster.
func (c *Cluster) Version() (string, error) {
	dc, err := c.DiscoveryClient()
	if err != nil {
		return "", err
	}
	serverVersion, err := dc.ServerVersion()
	if err != nil {
		return "", err
	}
	return fmt.Sprint(serverVersion), nil
}

// FromKubeConfig creates a Cluster from a kubeconfig.
func FromKubeConfig(ctx context.Context, kubeconfig, contextName string) (*Cluster, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	config, err := cc.ClientConfig()
	if err != nil {
		return nil, err
	}

	config = withConfigDefaults(config)

	return newCluster(ctx, cc, config)
}

// withConfigDefaults returns an extended rest.Config object with additional defaults applied
// See core_client.go#setConfigDefaults
func withConfigDefaults(inConfig *rest.Config) *rest.Config {
	config := rest.CopyConfig(inConfig)
	config.QPS = 30
	config.Burst = 100
	config.APIPath = "/api"
	if config.GroupVersion == nil || config.GroupVersion.Group != scheme.Scheme.PrioritizedVersionsForGroup("")[0].Group {
		gv := scheme.Scheme.PrioritizedVersionsForGroup("")[0]
		config.GroupVersion = &gv
	}
	codec := runtime.NoopEncoder{Decoder: scheme.Codecs.UniversalDecoder()}
	config.NegotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{Serializer: codec})

	return config
}
