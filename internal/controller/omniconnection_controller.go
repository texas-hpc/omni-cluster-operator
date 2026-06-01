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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/omniapi"
)

// OmniConnectionReconciler reconciles a OmniConnection object
type OmniConnectionReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	SecretReader client.Reader
	Omni         omniapi.Client
}

// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniconnections/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

func (r *OmniConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	connection := &omniv1alpha1.OmniConnection{}
	if err := r.Get(ctx, req.NamespacedName, connection); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	now := metav1.Now()
	output, err := r.omniClient().Ping(ctx, connection)
	statusErr := r.updateConnectionStatus(ctx, connection, func(status *omniv1alpha1.OmniConnectionStatus) {
		status.ObservedGeneration = connection.Generation
		status.ConnectionRef = connection.Name
		status.Endpoint = connection.Spec.Endpoint
		status.LastCheckTime = &now
		if err != nil {
			message := fmt.Sprintf("failed to connect to Omni: %v", err)
			omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReachable, metav1.ConditionFalse, connection.Generation, omniv1alpha1.ReasonConnectionFailed, message))
			omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, connection.Generation, omniv1alpha1.ReasonConnectionFailed, message))
			return
		}

		omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReachable, metav1.ConditionTrue, connection.Generation, omniv1alpha1.ReasonConnectionReady, output))
		omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionTrue, connection.Generation, omniv1alpha1.ReasonConnectionReady, output))
	})
	if statusErr != nil {
		return ctrl.Result{}, statusErr
	}

	if err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *OmniConnectionReconciler) omniClient() omniapi.Client {
	if r.Omni != nil {
		return r.Omni
	}

	secretReader := r.SecretReader
	if secretReader == nil {
		secretReader = r.Client
	}

	return &omniapi.RealClient{K8sClient: secretReader}
}

func (r *OmniConnectionReconciler) updateConnectionStatus(ctx context.Context, connection *omniv1alpha1.OmniConnection, mutate func(*omniv1alpha1.OmniConnectionStatus)) error {
	key := client.ObjectKeyFromObject(connection)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniConnection{}
		if err := r.Get(ctx, key, latest); err != nil {
			return err
		}

		mutate(&latest.Status)

		return r.Status().Update(ctx, latest)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniConnection{}, builder.WithPredicates(specOrDeletionChangedPredicate())).
		Named("omniconnection").
		Complete(r)
}
