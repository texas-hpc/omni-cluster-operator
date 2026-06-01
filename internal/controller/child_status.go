/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/addon"
	"github.com/texas-hpc/omni-cluster-operator/internal/cilium"
)

type childStatusClient struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r childStatusClient) clusterExists(ctx context.Context, namespace, name string) (bool, error) {
	cluster := &omniv1alpha1.OmniCluster{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func acceptedCondition(generation int64, clusterName string, exists bool) metav1.Condition {
	if exists {
		return omniv1alpha1.NewCondition(omniv1alpha1.ConditionAccepted, metav1.ConditionTrue, generation, omniv1alpha1.ReasonAccepted, fmt.Sprintf("attached to OmniCluster %q", clusterName))
	}

	return omniv1alpha1.NewCondition(omniv1alpha1.ConditionAccepted, metav1.ConditionFalse, generation, omniv1alpha1.ReasonMissingCluster, fmt.Sprintf("OmniCluster %q does not exist", clusterName))
}

func updateControlPlaneStatus(ctx context.Context, c client.Client, controlPlane *omniv1alpha1.OmniControlPlane, exists bool) error {
	key := client.ObjectKeyFromObject(controlPlane)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniControlPlane{}
		if err := c.Get(ctx, key, latest); err != nil {
			return err
		}

		originalStatus := latest.Status.DeepCopy()
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.ClusterRef = latest.Spec.ClusterRef.Name
		omniv1alpha1.SetCondition(&latest.Status.Conditions, acceptedCondition(latest.Generation, latest.Spec.ClusterRef.Name, exists))
		if reflect.DeepEqual(originalStatus, &latest.Status) {
			return nil
		}

		return c.Status().Update(ctx, latest)
	})
}

func updateWorkersStatus(ctx context.Context, c client.Client, workers *omniv1alpha1.OmniWorkers, exists bool) error {
	key := client.ObjectKeyFromObject(workers)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniWorkers{}
		if err := c.Get(ctx, key, latest); err != nil {
			return err
		}

		originalStatus := latest.Status.DeepCopy()
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.ClusterRef = latest.Spec.ClusterRef.Name
		latest.Status.WorkerSetName = latest.Spec.WorkerSetName
		if latest.Status.WorkerSetName == "" {
			latest.Status.WorkerSetName = latest.Name
		}
		omniv1alpha1.SetCondition(&latest.Status.Conditions, acceptedCondition(latest.Generation, latest.Spec.ClusterRef.Name, exists))
		if reflect.DeepEqual(originalStatus, &latest.Status) {
			return nil
		}

		return c.Status().Update(ctx, latest)
	})
}

func updateMachineStatus(ctx context.Context, c client.Client, machine *omniv1alpha1.OmniMachine, exists bool) error {
	key := client.ObjectKeyFromObject(machine)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniMachine{}
		if err := c.Get(ctx, key, latest); err != nil {
			return err
		}

		originalStatus := latest.Status.DeepCopy()
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.ClusterRef = latest.Spec.ClusterRef.Name
		latest.Status.MachineID = latest.Spec.MachineID
		if latest.Status.MachineID == "" {
			latest.Status.MachineID = latest.Name
		}
		omniv1alpha1.SetCondition(&latest.Status.Conditions, acceptedCondition(latest.Generation, latest.Spec.ClusterRef.Name, exists))
		if reflect.DeepEqual(originalStatus, &latest.Status) {
			return nil
		}

		return c.Status().Update(ctx, latest)
	})
}

func updateAddonStatus(ctx context.Context, c client.Client, item *omniv1alpha1.OmniClusterAddon, exists bool, rendered bool, manifestHash string, renderErr error) error {
	key := client.ObjectKeyFromObject(item)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniClusterAddon{}
		if err := c.Get(ctx, key, latest); err != nil {
			return err
		}

		valuesErr := validateAddonValues(latest)
		secretName := addon.RenderedManifestSecretName(latest)

		originalStatus := latest.Status.DeepCopy()
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.ClusterRef = latest.Spec.ClusterRef.Name
		latest.Status.Chart = latest.Spec.Helm.Chart
		latest.Status.ChartVersion = latest.Spec.Helm.Version
		latest.Status.ManifestName = addon.ManifestName(latest)
		latest.Status.RenderedManifestSecretRef = secretName
		latest.Status.RenderedManifestHash = manifestHash
		if rendered {
			now := metav1.Now()
			latest.Status.LastRenderTime = &now
		}

		omniv1alpha1.SetCondition(&latest.Status.Conditions, acceptedCondition(latest.Generation, latest.Spec.ClusterRef.Name, exists))
		switch {
		case valuesErr != nil:
			message := valuesErr.Error()
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionRendered, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonRenderFailed, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonRenderFailed, message))
		case renderErr != nil:
			message := renderErr.Error()
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionRendered, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonRenderFailed, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonRenderFailed, message))
		case !exists:
			message := fmt.Sprintf("OmniCluster %q does not exist", latest.Spec.ClusterRef.Name)
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionRendered, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonMissingCluster, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonMissingCluster, message))
		default:
			message := fmt.Sprintf("rendered Helm chart %q version %q into Secret %q", latest.Spec.Helm.Chart, latest.Spec.Helm.Version, secretName)
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionRendered, metav1.ConditionTrue, latest.Generation, omniv1alpha1.ReasonRendered, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionTrue, latest.Generation, omniv1alpha1.ReasonRendered, message))
		}

		if reflect.DeepEqual(originalStatus, &latest.Status) {
			return nil
		}

		return c.Status().Update(ctx, latest)
	})
}

func updateCiliumStatus(ctx context.Context, c client.Client, install *omniv1alpha1.OmniCilium, exists bool, rendered bool, manifestHash string, renderErr error) error {
	key := client.ObjectKeyFromObject(install)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniCilium{}
		if err := c.Get(ctx, key, latest); err != nil {
			return err
		}

		_, kubeProxyReplacement, valuesErr := cilium.Values(latest)
		secretName := cilium.RenderedManifestSecretName(latest)

		originalStatus := latest.Status.DeepCopy()
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.ClusterRef = latest.Spec.ClusterRef.Name
		latest.Status.ChartVersion = latest.Spec.ChartVersion
		latest.Status.ManifestName = cilium.ManifestName(latest)
		latest.Status.KubeProxyReplacement = kubeProxyReplacement
		latest.Status.RenderedManifestSecretRef = secretName
		latest.Status.RenderedManifestHash = manifestHash
		if rendered {
			now := metav1.Now()
			latest.Status.LastRenderTime = &now
		}

		omniv1alpha1.SetCondition(&latest.Status.Conditions, acceptedCondition(latest.Generation, latest.Spec.ClusterRef.Name, exists))
		switch {
		case valuesErr != nil:
			message := valuesErr.Error()
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionRendered, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonRenderFailed, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonRenderFailed, message))
		case renderErr != nil:
			message := renderErr.Error()
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionRendered, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonRenderFailed, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonRenderFailed, message))
		case !exists:
			message := fmt.Sprintf("OmniCluster %q does not exist", latest.Spec.ClusterRef.Name)
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionRendered, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonMissingCluster, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonMissingCluster, message))
		default:
			message := fmt.Sprintf("rendered Cilium chart %q into Secret %q", latest.Spec.ChartVersion, secretName)
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionRendered, metav1.ConditionTrue, latest.Generation, omniv1alpha1.ReasonRendered, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionTrue, latest.Generation, omniv1alpha1.ReasonRendered, message))
		}

		if reflect.DeepEqual(originalStatus, &latest.Status) {
			return nil
		}

		return c.Status().Update(ctx, latest)
	})
}

func validateAddonValues(item *omniv1alpha1.OmniClusterAddon) error {
	_, err := addon.Values(item)
	return err
}
