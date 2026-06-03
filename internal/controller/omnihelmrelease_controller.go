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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/helmrelease"
)

const helmReleaseRetryInterval = time.Minute

// HelmReleaseClient reconciles Helm releases against a workload cluster.
type HelmReleaseClient interface {
	Reconcile(context.Context, *omniv1alpha1.OmniHelmRelease, []byte) (*helmrelease.Result, error)
	Uninstall(context.Context, *omniv1alpha1.OmniHelmRelease, []byte) (*helmrelease.Result, error)
}

// OmniHelmReleaseReconciler reconciles an OmniHelmRelease object.
type OmniHelmReleaseReconciler struct {
	client.Client
	SecretReader client.Reader
	Helm         HelmReleaseClient
}

// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnihelmreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnihelmreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnihelmreleases/finalizers,verbs=update
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *OmniHelmReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	item := &omniv1alpha1.OmniHelmRelease{}
	if err := r.Get(ctx, req.NamespacedName, item); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if !item.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, item)
	}

	result, done, err := r.ensureFinalizer(ctx, item)
	if done || err != nil {
		return result, err
	}

	if err := helmReleaseSpecError(item); err != nil {
		statusErr := updateHelmReleaseStatus(ctx, r.Client, item, helmReleaseStatusUpdate{
			reason:  omniv1alpha1.ReasonValidationFailed,
			message: err.Error(),
			err:     err,
		})

		return ctrl.Result{}, statusErr
	}

	exists, err := (childStatusClient{Client: r.Client}).clusterExists(ctx, item.Namespace, item.Spec.ClusterRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !exists {
		statusErr := updateHelmReleaseStatus(ctx, r.Client, item, helmReleaseStatusUpdate{
			acceptedKnown: true,
			clusterExists: false,
			reason:        omniv1alpha1.ReasonMissingCluster,
			message:       fmt.Sprintf("OmniCluster %q does not exist", item.Spec.ClusterRef.Name),
		})

		return ctrl.Result{RequeueAfter: helmReleaseRetryInterval}, statusErr
	}

	kubeconfig, result, done, err := r.kubeconfig(ctx, item)
	if done || err != nil {
		return result, err
	}

	helmResult, helmErr := r.helmClient().Reconcile(ctx, item, kubeconfig)
	statusErr := updateHelmReleaseStatus(ctx, r.Client, item, helmReleaseStatusUpdate{
		acceptedKnown: true,
		clusterExists: true,
		result:        helmResult,
		err:           helmErr,
	})
	if statusErr != nil {
		return ctrl.Result{}, statusErr
	}

	return ctrl.Result{}, helmErr
}

func (r *OmniHelmReleaseReconciler) reconcileDelete(ctx context.Context, item *omniv1alpha1.OmniHelmRelease) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(item, omniv1alpha1.Finalizer) {
		return ctrl.Result{}, nil
	}

	if helmrelease.DeletionPolicy(item) == omniv1alpha1.HelmReleaseDeletionPolicyOrphan {
		return ctrl.Result{}, r.removeFinalizer(ctx, item)
	}

	kubeconfig, result, done, err := r.kubeconfig(ctx, item)
	if done || err != nil {
		return result, err
	}

	helmResult, helmErr := r.helmClient().Uninstall(ctx, item, kubeconfig)
	statusErr := updateHelmReleaseStatus(ctx, r.Client, item, helmReleaseStatusUpdate{
		acceptedKnown: true,
		clusterExists: true,
		result:        helmResult,
		err:           helmErr,
	})
	if statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	if helmErr != nil {
		return ctrl.Result{}, helmErr
	}

	return ctrl.Result{}, r.removeFinalizer(ctx, item)
}

func (r *OmniHelmReleaseReconciler) ensureFinalizer(ctx context.Context, item *omniv1alpha1.OmniHelmRelease) (ctrl.Result, bool, error) {
	if controllerutil.ContainsFinalizer(item, omniv1alpha1.Finalizer) {
		return ctrl.Result{}, false, nil
	}

	controllerutil.AddFinalizer(item, omniv1alpha1.Finalizer)
	if err := r.Update(ctx, item); err != nil {
		return ctrl.Result{}, true, err
	}

	return ctrl.Result{Requeue: true}, true, nil
}

func (r *OmniHelmReleaseReconciler) removeFinalizer(ctx context.Context, item *omniv1alpha1.OmniHelmRelease) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniHelmRelease{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(item), latest); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}

			return err
		}

		controllerutil.RemoveFinalizer(latest, omniv1alpha1.Finalizer)

		return r.Update(ctx, latest)
	})
}

func (r *OmniHelmReleaseReconciler) kubeconfig(ctx context.Context, item *omniv1alpha1.OmniHelmRelease) ([]byte, ctrl.Result, bool, error) {
	secret := &corev1.Secret{}
	secretReader := r.SecretReader
	if secretReader == nil {
		secretReader = r.Client
	}
	key := client.ObjectKey{Namespace: item.Namespace, Name: item.Spec.KubeconfigSecretRef.Name}
	if err := secretReader.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			statusErr := updateHelmReleaseStatus(ctx, r.Client, item, helmReleaseStatusUpdate{
				acceptedKnown: true,
				clusterExists: true,
				reason:        omniv1alpha1.ReasonMissingSecret,
				message:       fmt.Sprintf("kubeconfig Secret %q does not exist", key.String()),
			})

			return nil, ctrl.Result{RequeueAfter: helmReleaseRetryInterval}, true, statusErr
		}

		return nil, ctrl.Result{}, true, err
	}

	secretKey := helmrelease.KubeconfigSecretKey(item)
	kubeconfig := secret.Data[secretKey]
	if len(kubeconfig) == 0 {
		err := fmt.Errorf("kubeconfig Secret %q does not contain key %q", key.String(), secretKey)
		statusErr := updateHelmReleaseStatus(ctx, r.Client, item, helmReleaseStatusUpdate{
			acceptedKnown: true,
			clusterExists: true,
			reason:        omniv1alpha1.ReasonMissingSecret,
			message:       err.Error(),
			err:           err,
		})

		return nil, ctrl.Result{RequeueAfter: helmReleaseRetryInterval}, true, statusErr
	}
	if _, err := clientcmd.Load(kubeconfig); err != nil {
		loadErr := fmt.Errorf("kubeconfig Secret %q key %q is invalid: %w", key.String(), secretKey, err)
		statusErr := updateHelmReleaseStatus(ctx, r.Client, item, helmReleaseStatusUpdate{
			acceptedKnown: true,
			clusterExists: true,
			reason:        omniv1alpha1.ReasonValidationFailed,
			message:       loadErr.Error(),
			err:           loadErr,
		})

		return nil, ctrl.Result{RequeueAfter: helmReleaseRetryInterval}, true, statusErr
	}

	return kubeconfig, ctrl.Result{}, false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniHelmReleaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniHelmRelease{}, builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniCluster{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return helmReleaseRequestsForCluster(ctx, r.Client, object)
		}), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return helmReleaseRequestsForSecret(ctx, r.Client, object)
		})).
		Named("omnihelmrelease").
		Complete(r)
}

func (r *OmniHelmReleaseReconciler) helmClient() HelmReleaseClient {
	if r.Helm != nil {
		return r.Helm
	}

	return helmrelease.Client{}
}

func helmReleaseSpecError(item *omniv1alpha1.OmniHelmRelease) error {
	if strings.TrimSpace(item.Spec.ClusterRef.Name) == "" {
		return fmt.Errorf("clusterRef.name is required")
	}
	if strings.TrimSpace(item.Spec.KubeconfigSecretRef.Name) == "" {
		return fmt.Errorf("kubeconfigSecretRef.name is required")
	}
	if item.Spec.KubeconfigSecretRef.Key != "" && strings.TrimSpace(item.Spec.KubeconfigSecretRef.Key) == "" {
		return fmt.Errorf("kubeconfigSecretRef.key must not be blank")
	}
	if strings.TrimSpace(helmrelease.ReleaseName(item)) == "" {
		return fmt.Errorf("releaseName is required")
	}
	if strings.TrimSpace(helmrelease.Namespace(item)) == "" {
		return fmt.Errorf("namespace is required")
	}
	if strings.TrimSpace(item.Spec.Chart.Repository) == "" {
		return fmt.Errorf("chart.repository is required")
	}
	if strings.TrimSpace(item.Spec.Chart.Chart) == "" {
		return fmt.Errorf("chart.chart is required")
	}
	if strings.TrimSpace(item.Spec.Chart.Version) == "" {
		return fmt.Errorf("chart.version is required")
	}
	if item.Spec.Timeout != nil && item.Spec.Timeout.Duration <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}
	if item.Spec.MaxHistory < 0 {
		return fmt.Errorf("maxHistory must not be negative")
	}
	switch helmrelease.DeletionPolicy(item) {
	case omniv1alpha1.HelmReleaseDeletionPolicyUninstall, omniv1alpha1.HelmReleaseDeletionPolicyOrphan:
	default:
		return fmt.Errorf("deletionPolicy must be Uninstall or Orphan")
	}
	if _, err := helmrelease.Values(item); err != nil {
		return err
	}

	return nil
}

type helmReleaseStatusUpdate struct {
	acceptedKnown bool
	clusterExists bool
	result        *helmrelease.Result
	err           error
	reason        string
	message       string
}

func updateHelmReleaseStatus(ctx context.Context, c client.Client, item *omniv1alpha1.OmniHelmRelease, input helmReleaseStatusUpdate) error {
	key := client.ObjectKeyFromObject(item)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniHelmRelease{}
		if err := c.Get(ctx, key, latest); err != nil {
			return err
		}

		originalStatus := latest.Status.DeepCopy()
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.ClusterRef = latest.Spec.ClusterRef.Name
		latest.Status.KubeconfigSecretRef = latest.Spec.KubeconfigSecretRef.Name
		latest.Status.KubeconfigSecretKey = helmrelease.KubeconfigSecretKey(latest)
		latest.Status.ReleaseName = helmrelease.ReleaseName(latest)
		latest.Status.Namespace = helmrelease.Namespace(latest)
		latest.Status.Chart = latest.Spec.Chart.Chart
		latest.Status.ChartVersion = latest.Spec.Chart.Version
		if input.result != nil {
			latest.Status.LastAction = input.result.Action
			latest.Status.ReleaseName = input.result.ReleaseName
			latest.Status.Namespace = input.result.Namespace
			latest.Status.Chart = input.result.Chart
			latest.Status.ChartVersion = input.result.ChartVersion
			latest.Status.ReleaseRevision = int64(input.result.Revision)
			latest.Status.ReleaseStatus = input.result.Status
		}

		now := metav1.Now()
		if input.result != nil || input.err != nil || input.reason != "" {
			latest.Status.LastAttemptTime = &now
		}
		if input.err == nil && input.result != nil {
			latest.Status.LastSuccessTime = &now
			latest.Status.LastError = ""
		} else if input.err != nil {
			latest.Status.LastError = input.err.Error()
		} else if input.message != "" {
			latest.Status.LastError = input.message
		}

		if input.acceptedKnown {
			omniv1alpha1.SetCondition(&latest.Status.Conditions, acceptedCondition(latest.Generation, latest.Spec.ClusterRef.Name, input.clusterExists))
		}

		reason, message := helmReleaseConditionReasonMessage(latest, input)
		ready := metav1.ConditionFalse
		released := metav1.ConditionFalse
		if input.err == nil && input.result != nil && input.result.Status == helmrelease.StatusDeployed {
			ready = metav1.ConditionTrue
			released = metav1.ConditionTrue
		}
		if input.err == nil && input.result != nil && input.result.Action == helmrelease.ActionUninstall {
			ready = metav1.ConditionFalse
			released = metav1.ConditionFalse
		}
		omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReleased, released, latest.Generation, reason, message))
		omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, ready, latest.Generation, reason, message))

		if reflect.DeepEqual(originalStatus, &latest.Status) {
			return nil
		}

		return c.Status().Update(ctx, latest)
	})
}

func helmReleaseConditionReasonMessage(item *omniv1alpha1.OmniHelmRelease, input helmReleaseStatusUpdate) (string, string) {
	if input.reason != "" {
		return input.reason, input.message
	}
	if input.err != nil {
		message := input.err.Error()
		if input.result != nil && input.result.Action != "" {
			message = fmt.Sprintf("Helm %s failed for release %q: %v", strings.ToLower(input.result.Action), helmrelease.ReleaseName(item), input.err)
		}

		return omniv1alpha1.ReasonReconcileFailed, message
	}
	if input.result == nil {
		return omniv1alpha1.ReasonReconcileFailed, "Helm release has not been reconciled"
	}

	switch input.result.Action {
	case helmrelease.ActionInstall:
		return omniv1alpha1.ReasonHelmInstalled, fmt.Sprintf("installed Helm release %q revision %d", input.result.ReleaseName, input.result.Revision)
	case helmrelease.ActionUpgrade:
		return omniv1alpha1.ReasonHelmUpgraded, fmt.Sprintf("upgraded Helm release %q to revision %d", input.result.ReleaseName, input.result.Revision)
	case helmrelease.ActionUninstall:
		return omniv1alpha1.ReasonHelmUninstalled, fmt.Sprintf("uninstalled Helm release %q", input.result.ReleaseName)
	default:
		return omniv1alpha1.ReasonReconcileFailed, fmt.Sprintf("reconciled Helm release %q", input.result.ReleaseName)
	}
}

func helmReleaseRequestsForSecret(ctx context.Context, c client.Client, object client.Object) []ctrl.Request {
	releaseList := &omniv1alpha1.OmniHelmReleaseList{}
	if err := c.List(ctx, releaseList, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, item := range releaseList.Items {
		if item.Spec.KubeconfigSecretRef.Name == object.GetName() {
			requests = append(requests, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: item.Namespace, Name: item.Name}})
		}
	}

	return sortRequests(requests)
}
