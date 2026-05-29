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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

// OmniWorkersReconciler reconciles a OmniWorkers object
type OmniWorkersReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=omni.texas-hpc.org,resources=omniworkers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texas-hpc.org,resources=omniworkers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texas-hpc.org,resources=omniworkers/finalizers,verbs=update
// +kubebuilder:rbac:groups=omni.texas-hpc.org,resources=omniclusters,verbs=get;list;watch

func (r *OmniWorkersReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	workers := &omniv1alpha1.OmniWorkers{}
	if err := r.Get(ctx, req.NamespacedName, workers); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	exists, err := (childStatusClient{Client: r.Client, Scheme: r.Scheme}).clusterExists(ctx, workers.Namespace, workers.Spec.ClusterRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, updateWorkersStatus(ctx, r.Client, workers, exists)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniWorkersReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniWorkers{}).
		Named("omniworkers").
		Complete(r)
}
