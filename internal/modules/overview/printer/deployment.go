/*
Copyright (c) 2019 VMware, Inc. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package printer

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware/octant/pkg/store"
	"github.com/vmware/octant/pkg/view/component"
)

// DeploymentListHandler is a printFunc that lists deployments
func DeploymentListHandler(_ context.Context, list *appsv1.DeploymentList, opts Options) (component.Component, error) {
	if list == nil {
		return nil, errors.New("nil list")
	}

	cols := component.NewTableCols("Name", "Labels", "Status", "Age", "Containers", "Selector")
	tbl := component.NewTable("Deployments", cols)

	for _, d := range list.Items {
		row := component.TableRow{}
		nameLink, err := opts.Link.ForObject(&d, d.Name)
		if err != nil {
			return nil, err
		}

		row["Name"] = nameLink
		row["Labels"] = component.NewLabels(d.Labels)

		status := fmt.Sprintf("%d/%d", d.Status.AvailableReplicas, d.Status.AvailableReplicas+d.Status.UnavailableReplicas)
		row["Status"] = component.NewText(status)

		ts := d.CreationTimestamp.Time
		row["Age"] = component.NewTimestamp(ts)

		containers := component.NewContainers()
		for _, c := range d.Spec.Template.Spec.Containers {
			containers.Add(c.Name, c.Image)
		}
		row["Containers"] = containers
		row["Selector"] = printSelector(d.Spec.Selector)

		tbl.Add(row)
	}
	return tbl, nil
}

// DeploymentHandler is a printFunc that prints a Deployments.
func DeploymentHandler(ctx context.Context, deployment *appsv1.Deployment, options Options) (component.Component, error) {
	o := NewObject(deployment)

	deployConfigGen := NewDeploymentConfiguration(deployment)
	configSummary, err := deployConfigGen.Create()
	if err != nil {
		return nil, err
	}

	deploySummaryGen := NewDeploymentStatus(deployment)
	statusSummary, err := deploySummaryGen.Create()
	if err != nil {
		return nil, err
	}

	o.RegisterConfig(configSummary)
	o.RegisterItems([]ItemDescriptor{
		{
			Func: func() (component.Component, error) {
				return deploymentPods(ctx, deployment, options)
			},
			Width: component.WidthFull,
		},
		{
			Width: component.WidthQuarter,
			Func: func() (component.Component, error) {
				return statusSummary, nil
			},
		},
	}...)
	o.EnablePodTemplate(deployment.Spec.Template)
	o.EnableEvents()

	return o.ToComponent(ctx, options)
}

type actionGeneratorFunction func(*appsv1.Deployment) []component.Action

// DeploymentConfiguration generates deployment configuration.
type DeploymentConfiguration struct {
	deployment       *appsv1.Deployment
	actionGenerators []actionGeneratorFunction
}

// NewDeploymentConfiguration creates an instance of DeploymentConfiguration.
func NewDeploymentConfiguration(d *appsv1.Deployment) *DeploymentConfiguration {
	return &DeploymentConfiguration{
		deployment:       d,
		actionGenerators: []actionGeneratorFunction{editDeploymentAction},
	}
}

// Create creates a deployment configuration summary.
func (dc *DeploymentConfiguration) Create() (*component.Summary, error) {
	if dc.deployment == nil {
		return nil, errors.New("deployment is nil")
	}

	sections := make([]component.SummarySection, 0)

	strategyType := dc.deployment.Spec.Strategy.Type
	sections = append(sections, component.SummarySection{
		Header:  "Deployment Strategy",
		Content: component.NewText(string(strategyType)),
	})

	switch strategyType {
	case appsv1.RollingUpdateDeploymentStrategyType:
		rollingUpdate := dc.deployment.Spec.Strategy.RollingUpdate
		if rollingUpdate == nil {
			return nil, errors.Errorf("deployment strategy type is RollingUpdate, but configuration is nil")
		}

		rollingUpdateText := fmt.Sprintf("Max Surge %s, Max Unavailable %s",
			rollingUpdate.MaxSurge.String(),
			rollingUpdate.MaxUnavailable.String(),
		)

		sections = append(sections, component.SummarySection{
			Header:  "Rolling Update Strategy",
			Content: component.NewText(rollingUpdateText),
		})

		if selector := dc.deployment.Spec.Selector; selector != nil {
			var selectors []component.Selector

			for _, lsr := range selector.MatchExpressions {
				o, err := component.MatchOperator(string(lsr.Operator))
				if err != nil {
					return nil, err
				}

				es := component.NewExpressionSelector(lsr.Key, o, lsr.Values)
				selectors = append(selectors, es)
			}

			for k, v := range selector.MatchLabels {
				ls := component.NewLabelSelector(k, v)
				selectors = append(selectors, ls)
			}

			sections = append(sections, component.SummarySection{
				Header:  "Selectors",
				Content: component.NewSelectors(selectors),
			})
		}

		minReadySeconds := fmt.Sprintf("%d", dc.deployment.Spec.MinReadySeconds)
		sections = append(sections, component.SummarySection{
			Header:  "Min Ready Seconds",
			Content: component.NewText(minReadySeconds),
		})

		if rhl := dc.deployment.Spec.RevisionHistoryLimit; rhl != nil {
			revisionHistoryLimit := fmt.Sprintf("%d", *rhl)
			sections = append(sections, component.SummarySection{
				Header:  "Revision History Limit",
				Content: component.NewText(revisionHistoryLimit),
			})
		}
	}

	var replicas int32
	if dc.deployment.Spec.Replicas != nil {
		replicas = *dc.deployment.Spec.Replicas
	}

	sections = append(sections, component.SummarySection{
		Header:  "Replicas",
		Content: component.NewText(fmt.Sprintf("%d", replicas)),
	})

	summary := component.NewSummary("Configuration", sections...)

	for _, generator := range dc.actionGenerators {
		actions := generator(dc.deployment)
		for _, action := range actions {
			summary.AddAction(action)
		}
	}

	return summary, nil
}

// DeploymentStatus generates deployment status.
type DeploymentStatus struct {
	deployment *appsv1.Deployment
}

// NewDeploymentStatus creates an instance of DeploymentStatus.
func NewDeploymentStatus(d *appsv1.Deployment) *DeploymentStatus {
	return &DeploymentStatus{
		deployment: d,
	}
}

// Create generates a deployment status quadrant.
func (ds *DeploymentStatus) Create() (*component.Quadrant, error) {
	if ds.deployment == nil {
		return nil, errors.New("deployment is nil")
	}

	status := ds.deployment.Status

	quadrant := component.NewQuadrant("Status")
	if err := quadrant.Set(component.QuadNW, "Updated", fmt.Sprintf("%d", status.UpdatedReplicas)); err != nil {
		return nil, errors.New("unable to set quadrant nw")
	}
	if err := quadrant.Set(component.QuadNE, "Total", fmt.Sprintf("%d", status.Replicas)); err != nil {
		return nil, errors.New("unable to set quadrant ne")
	}
	if err := quadrant.Set(component.QuadSW, "Unavailable", fmt.Sprintf("%d", status.UnavailableReplicas)); err != nil {
		return nil, errors.New("unable to set quadrant sw")
	}
	if err := quadrant.Set(component.QuadSE, "Available", fmt.Sprintf("%d", status.AvailableReplicas)); err != nil {
		return nil, errors.New("unable to set quadrant se")
	}

	return quadrant, nil
}

func deploymentPods(ctx context.Context, deployment *appsv1.Deployment, options Options) (component.Component, error) {
	if deployment == nil {
		return nil, errors.New("deployment is nil")
	}

	objectStore := options.DashConfig.ObjectStore()

	if objectStore == nil {
		return nil, errors.New("objectStore is nil")
	}

	selector := labels.Set(deployment.Spec.Template.ObjectMeta.Labels)

	key := store.Key{
		Namespace:  deployment.Namespace,
		APIVersion: "v1",
		Kind:       "Pod",
		Selector:   &selector,
	}

	list, err := objectStore.List(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "list all objects for key %s", key)
	}

	podList := &corev1.PodList{}
	for _, u := range list {
		pod := &corev1.Pod{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, pod)
		if err != nil {
			return nil, err
		}

		if err := copyObjectMeta(pod, u); err != nil {
			return nil, errors.Wrap(err, "copy object metadata")
		}

		podList.Items = append(podList.Items, *pod)
	}

	options.DisableLabels = true
	return PodListHandler(ctx, podList, options)
}

func editDeploymentAction(deployment *appsv1.Deployment) []component.Action {
	replicas := deployment.Spec.Replicas
	if replicas == nil {
		return []component.Action{}
	}

	gvk := deployment.GroupVersionKind()
	group := gvk.Group
	version := gvk.Version
	kind := gvk.Kind

	action := component.Action{
		Name:  "Edit",
		Title: "Deployment Editor",
		Form: component.Form{
			Fields: []component.FormField{
				component.NewFormFieldNumber("Replicas", "replicas", fmt.Sprintf("%d", *replicas)),
				component.NewFormFieldHidden("group", group),
				component.NewFormFieldHidden("version", version),
				component.NewFormFieldHidden("kind", kind),
				component.NewFormFieldHidden("name", deployment.Name),
				component.NewFormFieldHidden("namespace", deployment.Namespace),
				component.NewFormFieldHidden("action", "deployment/configuration"),
			},
		},
	}

	return []component.Action{action}

}
