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
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

type childStatusClient struct {
	client.Client
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
