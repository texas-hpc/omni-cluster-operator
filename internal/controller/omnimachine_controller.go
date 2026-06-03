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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

// OmniMachineReconciler reconciles a OmniMachine object
type OmniMachineReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnimachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnimachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnimachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters,verbs=get;list;watch

func (r *OmniMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	machine := &omniv1alpha1.OmniMachine{}
	if err := r.Get(ctx, req.NamespacedName, machine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	exists, err := (childStatusClient{Client: r.Client}).clusterExists(ctx, machine.Namespace, machine.Spec.ClusterRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, updateMachineStatus(ctx, r.Client, machine, exists)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniMachine{}, builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniCluster{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return machineRequestsForCluster(ctx, r.Client, object)
		}), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Named("omnimachine").
		Complete(r)
}
