/*
Copyright (c) 2019 VMware, Inc. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package queryer

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kLabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/pkg/apis/apps"
	"k8s.io/kubernetes/pkg/apis/batch"
	"k8s.io/kubernetes/pkg/apis/core"

	dashstrings "github.com/vmware/octant/internal/util/strings"
	"github.com/vmware/octant/pkg/store"
)

//go:generate mockgen -source=queryer.go -destination=./fake/mock_queryer.go -package=fake github.com/vmware/octant/internal/queryer Queryer
//go:generate mockgen -source=../../vendor/k8s.io/client-go/discovery/discovery_client.go -imports=openapi_v2=github.com/googleapis/gnostic/OpenAPIv2 -destination=./fake/mock_discovery.go -package=fake k8s.io/client-go/discovery DiscoveryInterface

type Queryer interface {
	Children(ctx context.Context, object metav1.Object) ([]runtime.Object, error)
	Events(ctx context.Context, object metav1.Object) ([]*corev1.Event, error)
	IngressesForService(ctx context.Context, service *corev1.Service) ([]*extv1beta1.Ingress, error)
	OwnerReference(ctx context.Context, namespace string, ownerReference metav1.OwnerReference) (runtime.Object, error)
	PodsForService(ctx context.Context, service *corev1.Service) ([]*corev1.Pod, error)
	ServicesForIngress(ctx context.Context, ingress *extv1beta1.Ingress) ([]*corev1.Service, error)
	ServicesForPod(ctx context.Context, pod *corev1.Pod) ([]*corev1.Service, error)
	ServiceAccountForPod(ctx context.Context, pod *corev1.Pod) (*corev1.ServiceAccount, error)
}

type childrenCache struct {
	children map[types.UID][]runtime.Object
	mu       sync.RWMutex
}

func initChildrenCache() *childrenCache {
	return &childrenCache{
		children: make(map[types.UID][]runtime.Object),
	}
}

func (c *childrenCache) get(key types.UID) ([]runtime.Object, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.children[key]
	return v, ok
}

func (c *childrenCache) set(key types.UID, value []runtime.Object) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.children[key] = value
}

type ownerCache struct {
	owner map[store.Key]runtime.Object
	mu    sync.Mutex
}

func initOwnerCache() *ownerCache {
	return &ownerCache{
		owner: make(map[store.Key]runtime.Object),
	}
}

func (c *ownerCache) set(key store.Key, value runtime.Object) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if value == nil {
		return
	}

	c.owner[key] = value
}

func (c *ownerCache) get(key store.Key) (runtime.Object, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.owner[key]
	return v, ok
}

type podsForServicesCache struct {
	podsForServices map[types.UID][]*corev1.Pod
	mu              sync.Mutex
}

func initPodsForServicesCache() *podsForServicesCache {
	return &podsForServicesCache{
		podsForServices: make(map[types.UID][]*corev1.Pod),
	}
}

func (c *podsForServicesCache) set(key types.UID, value []*corev1.Pod) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.podsForServices[key] = value
}

func (c *podsForServicesCache) get(key types.UID) ([]*corev1.Pod, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.podsForServices[key]
	return v, ok
}

type ObjectStoreQueryer struct {
	objectStore     store.Store
	discoveryClient discovery.DiscoveryInterface

	children        *childrenCache
	podsForServices *podsForServicesCache
	owner           *ownerCache

	// mu sync.Mutex
}

var _ Queryer = (*ObjectStoreQueryer)(nil)

func New(o store.Store, discoveryClient discovery.DiscoveryInterface) *ObjectStoreQueryer {
	return &ObjectStoreQueryer{
		objectStore:     o,
		discoveryClient: discoveryClient,

		children:        initChildrenCache(),
		podsForServices: initPodsForServicesCache(),
		owner:           initOwnerCache(),
	}
}

func (osq *ObjectStoreQueryer) Children(ctx context.Context, owner metav1.Object) ([]runtime.Object, error) {
	if owner == nil {
		return nil, errors.New("owner is nil")
	}

	ctx, span := trace.StartSpan(ctx, "queryer:Children")
	defer span.End()

	stored, ok := osq.children.get(owner.GetUID())

	if ok {
		return stored, nil
	}

	var children []runtime.Object

	ch := make(chan runtime.Object)
	childrenProcessed := make(chan bool, 1)
	go func() {
		for child := range ch {
			if child == nil {
				continue
			}
			children = append(children, child)
		}
		childrenProcessed <- true
	}()

	resourceLists, err := osq.discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, err
	}

	var g errgroup.Group

	for resourceListIndex := range resourceLists {
		resourceList := resourceLists[resourceListIndex]
		if resourceList == nil {
			continue
		}

		for i := range resourceList.APIResources {
			apiResource := resourceList.APIResources[i]
			if !apiResource.Namespaced {
				continue
			}

			gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
			if err != nil {
				return nil, err
			}

			if gv.Group == "apps" && apiResource.Kind == "ReplicaSet" {
				// skip looking for apps/* ReplicaSet because extensions/v1beta1 ReplicaSet
				// is Octant's current default.
				continue
			}

			key := store.Key{
				Namespace:  owner.GetNamespace(),
				APIVersion: resourceList.GroupVersion,
				Kind:       apiResource.Kind,
			}

			if osq.canList(apiResource) {
				continue
			}

			g.Go(func() error {
				objects, err := osq.objectStore.List(ctx, key)
				if err != nil {
					return errors.Wrapf(err, "unable to retrieve %+v", key)
				}

				for _, object := range objects {
					if metav1.IsControlledBy(object, owner) {
						ch <- object
					}
				}

				return nil
			})
		}
	}

	if err := g.Wait(); err != nil {
		return nil, errors.Wrap(err, "find children")
	}

	close(ch)
	<-childrenProcessed
	close(childrenProcessed)

	osq.children.set(owner.GetUID(), children)

	return children, nil
}

func (osq *ObjectStoreQueryer) canList(apiResource metav1.APIResource) bool {
	return !dashstrings.Contains("watch", apiResource.Verbs) ||
		!dashstrings.Contains("list", apiResource.Verbs)
}

func (osq *ObjectStoreQueryer) Events(ctx context.Context, object metav1.Object) ([]*corev1.Event, error) {
	if object == nil {
		return nil, errors.New("object is nil")
	}

	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{Object: m}

	key := store.Key{
		Namespace:  u.GetNamespace(),
		APIVersion: "v1",
		Kind:       "Event",
	}

	allEvents, err := osq.objectStore.List(ctx, key)
	if err != nil {
		return nil, err
	}

	var events []*corev1.Event
	for _, unstructuredEvent := range allEvents {
		event := &corev1.Event{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredEvent.Object, event)
		if err != nil {
			return nil, err
		}

		involvedObject := event.InvolvedObject
		if involvedObject.Namespace == u.GetNamespace() &&
			involvedObject.APIVersion == u.GetAPIVersion() &&
			involvedObject.Kind == u.GetKind() &&
			involvedObject.Name == u.GetName() {
			events = append(events, event)
		}
	}

	return events, nil
}

func (osq *ObjectStoreQueryer) IngressesForService(ctx context.Context, service *corev1.Service) ([]*v1beta1.Ingress, error) {
	if service == nil {
		return nil, errors.New("nil service")
	}

	key := store.Key{
		Namespace:  service.Namespace,
		APIVersion: "extensions/v1beta1",
		Kind:       "Ingress",
	}
	ul, err := osq.objectStore.List(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "retrieving ingresses")
	}

	var results []*v1beta1.Ingress

	for _, u := range ul {
		ingress := &v1beta1.Ingress{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, ingress)
		if err != nil {
			return nil, errors.Wrap(err, "converting unstructured ingress")
		}
		if err = copyObjectMeta(ingress, u); err != nil {
			return nil, errors.Wrap(err, "copying object metadata")
		}
		backends := osq.listIngressBackends(*ingress)
		if !containsBackend(backends, service.Name) {
			continue
		}

		results = append(results, ingress)
	}
	return results, nil
}

func (osq *ObjectStoreQueryer) listIngressBackends(ingress v1beta1.Ingress) []extv1beta1.IngressBackend {
	var backends []v1beta1.IngressBackend

	if ingress.Spec.Backend != nil && ingress.Spec.Backend.ServiceName != "" {
		backends = append(backends, *ingress.Spec.Backend)
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.IngressRuleValue.HTTP == nil {
			continue
		}
		for _, p := range rule.IngressRuleValue.HTTP.Paths {
			if p.Backend.ServiceName == "" {
				continue
			}
			backends = append(backends, p.Backend)
		}
	}

	return backends
}

func (osq *ObjectStoreQueryer) OwnerReference(ctx context.Context, namespace string, ownerReference metav1.OwnerReference) (runtime.Object, error) {
	key := store.Key{
		Namespace:  namespace,
		APIVersion: ownerReference.APIVersion,
		Kind:       ownerReference.Kind,
		Name:       ownerReference.Name,
	}

	object, ok := osq.owner.get(key)
	if ok {
		return object, nil
	}

	owner, err := osq.objectStore.Get(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "get owner from store")
	}

	osq.owner.set(key, owner)

	return owner, nil
}

func (osq *ObjectStoreQueryer) PodsForService(ctx context.Context, service *corev1.Service) ([]*corev1.Pod, error) {
	if service == nil {
		return nil, errors.New("nil service")
	}

	stored, ok := osq.podsForServices.get(service.UID)
	if ok {
		return stored, nil
	}

	key := store.Key{
		Namespace:  service.Namespace,
		APIVersion: "v1",
		Kind:       "Pod",
	}

	selector, err := osq.getSelector(service)
	if err != nil {
		return nil, errors.Wrapf(err, "creating pod selector for service: %v", service.Name)
	}
	pods, err := osq.loadPods(ctx, key, selector)
	if err != nil {
		return nil, errors.Wrapf(err, "fetching pods for service: %v", service.Name)
	}

	osq.podsForServices.set(service.UID, pods)

	return pods, nil
}

func (osq *ObjectStoreQueryer) loadPods(ctx context.Context, key store.Key, labelSelector *metav1.LabelSelector) ([]*corev1.Pod, error) {
	objects, err := osq.objectStore.List(ctx, key)
	if err != nil {
		return nil, err
	}

	var list []*corev1.Pod

	for _, object := range objects {
		pod := &corev1.Pod{}
		if err := scheme.Scheme.Convert(object, pod, runtime.InternalGroupVersioner); err != nil {
			return nil, err
		}

		if err := copyObjectMeta(pod, object); err != nil {
			return nil, err
		}

		podSelector := &metav1.LabelSelector{
			MatchLabels: pod.GetLabels(),
		}

		selector, err := metav1.LabelSelectorAsSelector(labelSelector)
		if err != nil {
			return nil, err
		}

		if selector == nil || isEqualSelector(labelSelector, podSelector) || selector.Matches(kLabels.Set(pod.Labels)) {
			list = append(list, pod)
		}
	}

	return list, nil
}

func (osq *ObjectStoreQueryer) ServicesForIngress(ctx context.Context, ingress *extv1beta1.Ingress) ([]*corev1.Service, error) {
	if ingress == nil {
		return nil, errors.New("ingress is nil")
	}

	backends := osq.listIngressBackends(*ingress)
	var services []*corev1.Service
	for _, backend := range backends {
		key := store.Key{
			Namespace:  ingress.Namespace,
			APIVersion: "v1",
			Kind:       "Service",
			Name:       backend.ServiceName,
		}
		u, err := osq.objectStore.Get(ctx, key)
		if err != nil {
			return nil, errors.Wrapf(err, "retrieving service backend: %v", backend)
		}

		if u == nil {
			continue
		}

		svc := &corev1.Service{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, svc)
		if err != nil {
			return nil, errors.Wrap(err, "converting unstructured service")
		}
		if err := copyObjectMeta(svc, u); err != nil {
			return nil, errors.Wrap(err, "copying object metadata")
		}
		services = append(services, svc)
	}
	return services, nil
}

func (osq *ObjectStoreQueryer) ServicesForPod(ctx context.Context, pod *corev1.Pod) ([]*corev1.Service, error) {
	var results []*corev1.Service
	if pod == nil {
		return nil, errors.New("nil pod")
	}

	key := store.Key{
		Namespace:  pod.Namespace,
		APIVersion: "v1",
		Kind:       "Service",
	}
	ul, err := osq.objectStore.List(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "retrieving services")
	}
	for _, u := range ul {
		svc := &corev1.Service{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, svc)
		if err != nil {
			return nil, errors.Wrap(err, "converting unstructured service")
		}
		if err = copyObjectMeta(svc, u); err != nil {
			return nil, errors.Wrap(err, "copying object metadata")
		}
		labelSelector, err := osq.getSelector(svc)
		if err != nil {
			return nil, errors.Wrapf(err, "creating pod selector for service: %v", svc.Name)
		}
		selector, err := metav1.LabelSelectorAsSelector(labelSelector)
		if err != nil {
			return nil, errors.Wrap(err, "invalid selector")
		}

		if selector.Empty() || !selector.Matches(kLabels.Set(pod.Labels)) {
			continue
		}
		results = append(results, svc)
	}
	return results, nil
}

func (osq *ObjectStoreQueryer) ServiceAccountForPod(ctx context.Context, pod *corev1.Pod) (*corev1.ServiceAccount, error) {
	if pod == nil {
		return nil, errors.New("pod is nil")
	}

	if pod.Spec.ServiceAccountName == "" {
		return nil, nil
	}

	key := store.Key{
		Namespace:  pod.Namespace,
		APIVersion: "v1",
		Kind:       "ServiceAccount",
		Name:       pod.Spec.ServiceAccountName,
	}

	u, err := osq.objectStore.Get(ctx, key)
	if err != nil {
		return nil, errors.WithMessagef(err, "retrieve service account %q from namespace %q",
			key.Namespace, key.Namespace)
	}

	serviceAccount := &corev1.ServiceAccount{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, serviceAccount); err != nil {
		return nil, errors.WithMessage(err, "converting unstructured object to service account")
	}

	if err = copyObjectMeta(serviceAccount, u); err != nil {
		return nil, errors.Wrap(err, "copying object metadata")
	}

	return serviceAccount, nil

}

func (osq *ObjectStoreQueryer) getSelector(object runtime.Object) (*metav1.LabelSelector, error) {
	switch t := object.(type) {
	case *appsv1.DaemonSet:
		return t.Spec.Selector, nil
	case *appsv1.StatefulSet:
		return t.Spec.Selector, nil
	case *batchv1beta1.CronJob:
		return nil, nil
	case *corev1.ReplicationController:
		selector := &metav1.LabelSelector{
			MatchLabels: t.Spec.Selector,
		}
		return selector, nil
	case *v1beta1.ReplicaSet:
		return t.Spec.Selector, nil
	case *appsv1.ReplicaSet:
		return t.Spec.Selector, nil
	case *appsv1.Deployment:
		return t.Spec.Selector, nil
	case *corev1.Service:
		selector := &metav1.LabelSelector{
			MatchLabels: t.Spec.Selector,
		}
		return selector, nil
	case *apps.DaemonSet:
		return t.Spec.Selector, nil
	case *apps.StatefulSet:
		return t.Spec.Selector, nil
	case *batch.CronJob:
		return nil, nil
	case *core.ReplicationController:
		selector := &metav1.LabelSelector{
			MatchLabels: t.Spec.Selector,
		}
		return selector, nil
	case *apps.ReplicaSet:
		return t.Spec.Selector, nil
	case *apps.Deployment:
		return t.Spec.Selector, nil
	case *core.Service:
		selector := &metav1.LabelSelector{
			MatchLabels: t.Spec.Selector,
		}
		return selector, nil
	default:
		return nil, errors.Errorf("unable to retrieve selector for type %T", object)
	}
}

func copyObjectMeta(to interface{}, from *unstructured.Unstructured) error {
	object, ok := to.(metav1.Object)
	if !ok {
		return errors.Errorf("%T is not an object", to)
	}

	t, err := meta.TypeAccessor(object)
	if err != nil {
		return errors.Wrapf(err, "accessing type meta")
	}
	t.SetAPIVersion(from.GetAPIVersion())
	t.SetKind(from.GetObjectKind().GroupVersionKind().Kind)

	object.SetNamespace(from.GetNamespace())
	object.SetName(from.GetName())
	object.SetGenerateName(from.GetGenerateName())
	object.SetUID(from.GetUID())
	object.SetResourceVersion(from.GetResourceVersion())
	object.SetGeneration(from.GetGeneration())
	object.SetSelfLink(from.GetSelfLink())
	object.SetCreationTimestamp(from.GetCreationTimestamp())
	object.SetDeletionTimestamp(from.GetDeletionTimestamp())
	object.SetDeletionGracePeriodSeconds(from.GetDeletionGracePeriodSeconds())
	object.SetLabels(from.GetLabels())
	object.SetAnnotations(from.GetAnnotations())
	object.SetInitializers(from.GetInitializers())
	object.SetOwnerReferences(from.GetOwnerReferences())
	object.SetClusterName(from.GetClusterName())
	object.SetFinalizers(from.GetFinalizers())

	return nil
}

// extraKeys are keys that should be ignored in labels. These keys are added
// by tools or by Kubernetes itself.
var extraKeys = []string{
	"statefulset.kubernetes.io/pod-name",
	appsv1.DefaultDeploymentUniqueLabelKey,
	"controller-revision-hash",
	"pod-template-generation",
}

func isEqualSelector(s1, s2 *metav1.LabelSelector) bool {
	s1Copy := s1.DeepCopy()
	s2Copy := s2.DeepCopy()

	for _, key := range extraKeys {
		delete(s1Copy.MatchLabels, key)
		delete(s2Copy.MatchLabels, key)
	}

	return apiequality.Semantic.DeepEqual(s1Copy, s2Copy)
}

func containsBackend(lst []v1beta1.IngressBackend, s string) bool {
	for _, item := range lst {
		if item.ServiceName == s {
			return true
		}
	}
	return false
}
