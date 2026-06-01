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
	"sort"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

func controlPlaneRequestsForCluster(ctx context.Context, c client.Client, object client.Object) []reconcile.Request {
	controlPlanes := &omniv1alpha1.OmniControlPlaneList{}
	if err := c.List(ctx, controlPlanes, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, controlPlane := range controlPlanes.Items {
		if controlPlane.Spec.ClusterRef.Name == object.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: controlPlane.Namespace, Name: controlPlane.Name}})
		}
	}

	return sortRequests(requests)
}

func workersRequestsForCluster(ctx context.Context, c client.Client, object client.Object) []reconcile.Request {
	workersList := &omniv1alpha1.OmniWorkersList{}
	if err := c.List(ctx, workersList, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, workers := range workersList.Items {
		if workers.Spec.ClusterRef.Name == object.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: workers.Namespace, Name: workers.Name}})
		}
	}

	return sortRequests(requests)
}

func machineRequestsForCluster(ctx context.Context, c client.Client, object client.Object) []reconcile.Request {
	machineList := &omniv1alpha1.OmniMachineList{}
	if err := c.List(ctx, machineList, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, machine := range machineList.Items {
		if machine.Spec.ClusterRef.Name == object.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: machine.Namespace, Name: machine.Name}})
		}
	}

	return sortRequests(requests)
}

func ciliumRequestsForCluster(ctx context.Context, c client.Client, object client.Object) []reconcile.Request {
	ciliumList := &omniv1alpha1.OmniCiliumList{}
	if err := c.List(ctx, ciliumList, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, install := range ciliumList.Items {
		if install.Spec.ClusterRef.Name == object.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: install.Namespace, Name: install.Name}})
		}
	}

	return sortRequests(requests)
}

func kubeconfigExportRequestsForCluster(ctx context.Context, c client.Client, object client.Object) []reconcile.Request {
	exports := &omniv1alpha1.OmniKubeconfigExportList{}
	if err := c.List(ctx, exports, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, export := range exports.Items {
		if export.Spec.ClusterRef.Name == object.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: export.Namespace, Name: export.Name}})
		}
	}

	return sortRequests(requests)
}

func sortRequests(requests []reconcile.Request) []reconcile.Request {
	sort.Slice(requests, func(i, j int) bool {
		return strings.Compare(requests[i].String(), requests[j].String()) < 0
	})

	return requests
}
