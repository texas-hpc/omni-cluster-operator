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
	"github.com/texas-hpc/omni-cluster-operator/internal/omnitemplate"
	"github.com/texas-hpc/omni-cluster-operator/internal/secretsync"
)

const secretSyncRetryInterval = time.Minute

// SecretSyncClient syncs Secrets against a workload cluster.
type SecretSyncClient interface {
	Sync(context.Context, *omniv1alpha1.OmniSecretSync, *corev1.Secret, []byte, string) (*secretsync.Result, error)
	Delete(context.Context, *omniv1alpha1.OmniSecretSync, []byte, secretsync.Target) error
}

// OmniSecretSyncReconciler reconciles an OmniSecretSync object.
type OmniSecretSyncReconciler struct {
	client.Client
	SecretReader client.Reader
	SecretSync   SecretSyncClient
}

// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnisecretsyncs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnisecretsyncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnisecretsyncs/finalizers,verbs=update
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *OmniSecretSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	item := &omniv1alpha1.OmniSecretSync{}
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

	if err := secretSyncSpecError(item); err != nil {
		statusErr := updateSecretSyncStatus(ctx, r.Client, item, secretSyncStatusUpdate{
			reason: omniv1alpha1.ReasonValidationFailed,
			err:    err,
		})

		return ctrl.Result{}, statusErr
	}

	cluster, result, done, err := r.cluster(ctx, item)
	if done || err != nil {
		return result, err
	}
	clusterName := omnitemplate.ClusterName(cluster)

	source, result, done, err := r.sourceSecret(ctx, item)
	if done || err != nil {
		return result, err
	}

	kubeconfig, result, done, err := r.kubeconfig(ctx, item)
	if done || err != nil {
		return result, err
	}

	if err := r.deletePreviousTargetSecret(ctx, item, kubeconfig); err != nil {
		statusErr := updateSecretSyncStatus(ctx, r.Client, item, secretSyncStatusUpdate{
			acceptedKnown: true,
			clusterExists: true,
			reason:        omniv1alpha1.ReasonDeleteFailed,
			err:           err,
		})
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: secretSyncRetryInterval}, err
	}

	syncResult, syncErr := r.secretSyncClient().Sync(ctx, item, source, kubeconfig, clusterName)
	statusErr := updateSecretSyncStatus(ctx, r.Client, item, secretSyncStatusUpdate{
		acceptedKnown: true,
		clusterExists: true,
		result:        syncResult,
		err:           syncErr,
	})
	if statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	if syncErr != nil {
		return ctrl.Result{RequeueAfter: secretSyncRetryInterval}, syncErr
	}

	return ctrl.Result{}, nil
}

func (r *OmniSecretSyncReconciler) reconcileDelete(ctx context.Context, item *omniv1alpha1.OmniSecretSync) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(item, omniv1alpha1.Finalizer) {
		return ctrl.Result{}, nil
	}

	if item.Spec.DeletionPolicy == omniv1alpha1.SecretSyncDeletionPolicyOrphan {
		return ctrl.Result{}, r.removeFinalizer(ctx, item)
	}

	kubeconfig, result, done, err := r.kubeconfig(ctx, item)
	if done || err != nil {
		return result, err
	}

	target := secretsync.TargetForItem(item)
	if item.Status.TargetSecretRef != "" && item.Status.TargetNamespace != "" {
		target = secretsync.Target{Namespace: item.Status.TargetNamespace, Name: item.Status.TargetSecretRef}
	}
	deleteErr := r.secretSyncClient().Delete(ctx, item, kubeconfig, target)
	if deleteErr != nil {
		statusErr := updateSecretSyncStatus(ctx, r.Client, item, secretSyncStatusUpdate{
			acceptedKnown: true,
			clusterExists: true,
			reason:        omniv1alpha1.ReasonDeleteFailed,
			err:           deleteErr,
		})
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: secretSyncRetryInterval}, deleteErr
	}

	return ctrl.Result{}, r.removeFinalizer(ctx, item)
}

func (r *OmniSecretSyncReconciler) ensureFinalizer(ctx context.Context, item *omniv1alpha1.OmniSecretSync) (ctrl.Result, bool, error) {
	if controllerutil.ContainsFinalizer(item, omniv1alpha1.Finalizer) {
		return ctrl.Result{}, false, nil
	}

	controllerutil.AddFinalizer(item, omniv1alpha1.Finalizer)
	if err := r.Update(ctx, item); err != nil {
		return ctrl.Result{}, true, err
	}

	return ctrl.Result{Requeue: true}, true, nil
}

func (r *OmniSecretSyncReconciler) removeFinalizer(ctx context.Context, item *omniv1alpha1.OmniSecretSync) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniSecretSync{}
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

func (r *OmniSecretSyncReconciler) cluster(ctx context.Context, item *omniv1alpha1.OmniSecretSync) (*omniv1alpha1.OmniCluster, ctrl.Result, bool, error) {
	cluster := &omniv1alpha1.OmniCluster{}
	clusterKey := client.ObjectKey{Namespace: item.Namespace, Name: item.Spec.ClusterRef.Name}
	if err := r.Get(ctx, clusterKey, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			statusErr := updateSecretSyncStatus(ctx, r.Client, item, secretSyncStatusUpdate{
				acceptedKnown: true,
				clusterExists: false,
				reason:        omniv1alpha1.ReasonMissingCluster,
				message:       fmt.Sprintf("OmniCluster %q does not exist", item.Spec.ClusterRef.Name),
			})

			return nil, ctrl.Result{RequeueAfter: secretSyncRetryInterval}, true, statusErr
		}

		return nil, ctrl.Result{}, true, err
	}

	return cluster, ctrl.Result{}, false, nil
}

func (r *OmniSecretSyncReconciler) sourceSecret(ctx context.Context, item *omniv1alpha1.OmniSecretSync) (*corev1.Secret, ctrl.Result, bool, error) {
	secret := &corev1.Secret{}
	secretReader := r.secretReader()
	key := client.ObjectKey{Namespace: item.Namespace, Name: item.Spec.SourceSecretRef.Name}
	if err := secretReader.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			statusErr := updateSecretSyncStatus(ctx, r.Client, item, secretSyncStatusUpdate{
				acceptedKnown: true,
				clusterExists: true,
				reason:        omniv1alpha1.ReasonMissingSecret,
				message:       fmt.Sprintf("source Secret %q does not exist", key.String()),
			})

			return nil, ctrl.Result{RequeueAfter: secretSyncRetryInterval}, true, statusErr
		}

		return nil, ctrl.Result{}, true, err
	}

	return secret, ctrl.Result{}, false, nil
}

func (r *OmniSecretSyncReconciler) kubeconfig(ctx context.Context, item *omniv1alpha1.OmniSecretSync) ([]byte, ctrl.Result, bool, error) {
	secret := &corev1.Secret{}
	secretReader := r.secretReader()
	key := client.ObjectKey{Namespace: item.Namespace, Name: item.Spec.KubeconfigSecretRef.Name}
	if err := secretReader.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			statusErr := updateSecretSyncStatus(ctx, r.Client, item, secretSyncStatusUpdate{
				acceptedKnown: true,
				clusterExists: true,
				reason:        omniv1alpha1.ReasonMissingSecret,
				message:       fmt.Sprintf("kubeconfig Secret %q does not exist", key.String()),
			})

			return nil, ctrl.Result{RequeueAfter: secretSyncRetryInterval}, true, statusErr
		}

		return nil, ctrl.Result{}, true, err
	}

	secretKey := secretsync.KubeconfigSecretKey(item)
	kubeconfig := secret.Data[secretKey]
	if len(kubeconfig) == 0 {
		err := fmt.Errorf("kubeconfig Secret %q does not contain key %q", key.String(), secretKey)
		statusErr := updateSecretSyncStatus(ctx, r.Client, item, secretSyncStatusUpdate{
			acceptedKnown: true,
			clusterExists: true,
			reason:        omniv1alpha1.ReasonMissingSecret,
			err:           err,
		})

		return nil, ctrl.Result{RequeueAfter: secretSyncRetryInterval}, true, statusErr
	}
	if _, err := clientcmd.Load(kubeconfig); err != nil {
		loadErr := fmt.Errorf("kubeconfig Secret %q key %q is invalid: %w", key.String(), secretKey, err)
		statusErr := updateSecretSyncStatus(ctx, r.Client, item, secretSyncStatusUpdate{
			acceptedKnown: true,
			clusterExists: true,
			reason:        omniv1alpha1.ReasonValidationFailed,
			err:           loadErr,
		})

		return nil, ctrl.Result{RequeueAfter: secretSyncRetryInterval}, true, statusErr
	}

	return kubeconfig, ctrl.Result{}, false, nil
}

func (r *OmniSecretSyncReconciler) deletePreviousTargetSecret(ctx context.Context, item *omniv1alpha1.OmniSecretSync, kubeconfig []byte) error {
	if item.Spec.DeletionPolicy != omniv1alpha1.SecretSyncDeletionPolicyDelete ||
		item.Status.TargetSecretRef == "" ||
		item.Status.TargetNamespace == "" {
		return nil
	}

	current := secretsync.TargetForItem(item)
	previous := secretsync.Target{Namespace: item.Status.TargetNamespace, Name: item.Status.TargetSecretRef}
	if previous == current {
		return nil
	}

	return r.secretSyncClient().Delete(ctx, item, kubeconfig, previous)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniSecretSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniSecretSync{}, builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniCluster{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return secretSyncRequestsForCluster(ctx, r.Client, object)
		}), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return secretSyncRequestsForSecret(ctx, r.Client, object)
		})).
		Named("omnisecretsync").
		Complete(r)
}

func (r *OmniSecretSyncReconciler) secretReader() client.Reader {
	if r.SecretReader != nil {
		return r.SecretReader
	}

	return r.Client
}

func (r *OmniSecretSyncReconciler) secretSyncClient() SecretSyncClient {
	if r.SecretSync != nil {
		return r.SecretSync
	}

	return secretsync.Client{}
}

func secretSyncSpecError(item *omniv1alpha1.OmniSecretSync) error {
	if strings.TrimSpace(item.Spec.ClusterRef.Name) == "" {
		return fmt.Errorf("clusterRef.name is required")
	}
	if strings.TrimSpace(item.Spec.KubeconfigSecretRef.Name) == "" {
		return fmt.Errorf("kubeconfigSecretRef.name is required")
	}
	if item.Spec.KubeconfigSecretRef.Key != "" && strings.TrimSpace(item.Spec.KubeconfigSecretRef.Key) == "" {
		return fmt.Errorf("kubeconfigSecretRef.key must not be blank")
	}
	if strings.TrimSpace(item.Spec.SourceSecretRef.Name) == "" {
		return fmt.Errorf("sourceSecretRef.name is required")
	}
	if strings.TrimSpace(item.Spec.TargetSecretRef.Name) == "" {
		return fmt.Errorf("targetSecretRef.name is required")
	}
	if strings.TrimSpace(item.Spec.TargetSecretRef.Namespace) == "" {
		return fmt.Errorf("targetSecretRef.namespace is required")
	}
	if item.Spec.Type != "" && strings.TrimSpace(string(item.Spec.Type)) == "" {
		return fmt.Errorf("type must not be blank")
	}
	switch item.Spec.DeletionPolicy {
	case omniv1alpha1.SecretSyncDeletionPolicyDelete, omniv1alpha1.SecretSyncDeletionPolicyOrphan:
	default:
		return fmt.Errorf("deletionPolicy must be Delete or Orphan")
	}

	return nil
}

type secretSyncStatusUpdate struct {
	acceptedKnown bool
	clusterExists bool
	result        *secretsync.Result
	err           error
	reason        string
	message       string
}

func updateSecretSyncStatus(ctx context.Context, c client.Client, item *omniv1alpha1.OmniSecretSync, input secretSyncStatusUpdate) error {
	key := client.ObjectKeyFromObject(item)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniSecretSync{}
		if err := c.Get(ctx, key, latest); err != nil {
			return err
		}

		originalStatus := latest.Status.DeepCopy()
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.ClusterRef = latest.Spec.ClusterRef.Name
		latest.Status.KubeconfigSecretRef = latest.Spec.KubeconfigSecretRef.Name
		latest.Status.KubeconfigSecretKey = secretsync.KubeconfigSecretKey(latest)
		latest.Status.SourceSecretRef = latest.Spec.SourceSecretRef.Name
		latest.Status.TargetSecretRef = latest.Spec.TargetSecretRef.Name
		latest.Status.TargetNamespace = latest.Spec.TargetSecretRef.Namespace
		if latest.Spec.Type != "" {
			latest.Status.SecretType = string(latest.Spec.Type)
		}
		if input.result != nil {
			latest.Status.TargetSecretRef = input.result.Target.Name
			latest.Status.TargetNamespace = input.result.Target.Namespace
			latest.Status.SecretType = string(input.result.Type)
			latest.Status.SecretHash = input.result.Hash
		}

		now := metav1.Now()
		if input.result != nil || input.err != nil || input.reason != "" {
			latest.Status.LastAttemptTime = &now
		}
		if input.err == nil && input.result != nil {
			latest.Status.LastSyncTime = &now
			latest.Status.LastError = ""
		} else if input.err != nil {
			latest.Status.LastError = input.err.Error()
		} else if input.message != "" {
			latest.Status.LastError = input.message
		}

		if input.acceptedKnown {
			omniv1alpha1.SetCondition(&latest.Status.Conditions, acceptedCondition(latest.Generation, latest.Spec.ClusterRef.Name, input.clusterExists))
		}

		reason, message := secretSyncConditionReasonMessage(latest, input)
		ready := metav1.ConditionFalse
		synced := metav1.ConditionFalse
		if input.err == nil && input.result != nil {
			ready = metav1.ConditionTrue
			synced = metav1.ConditionTrue
		}
		omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionSynced, synced, latest.Generation, reason, message))
		omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, ready, latest.Generation, reason, message))

		if reflect.DeepEqual(originalStatus, &latest.Status) {
			return nil
		}

		return c.Status().Update(ctx, latest)
	})
}

func secretSyncConditionReasonMessage(item *omniv1alpha1.OmniSecretSync, input secretSyncStatusUpdate) (string, string) {
	if input.reason != "" {
		message := input.message
		if message == "" && input.err != nil {
			message = input.err.Error()
		}

		return input.reason, message
	}
	if input.err != nil {
		return omniv1alpha1.ReasonSyncFailed, input.err.Error()
	}
	if input.result == nil {
		return omniv1alpha1.ReasonSyncFailed, "Secret has not been synced"
	}

	return omniv1alpha1.ReasonSynced, fmt.Sprintf("synced Secret %q to workload Secret %q", item.Spec.SourceSecretRef.Name, client.ObjectKey{Namespace: input.result.Target.Namespace, Name: input.result.Target.Name}.String())
}

func secretSyncRequestsForCluster(ctx context.Context, c client.Client, object client.Object) []ctrl.Request {
	items := &omniv1alpha1.OmniSecretSyncList{}
	if err := c.List(ctx, items, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, item := range items.Items {
		if item.Spec.ClusterRef.Name == object.GetName() {
			requests = append(requests, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: item.Namespace, Name: item.Name}})
		}
	}

	return sortRequests(requests)
}

func secretSyncRequestsForSecret(ctx context.Context, c client.Client, object client.Object) []ctrl.Request {
	items := &omniv1alpha1.OmniSecretSyncList{}
	if err := c.List(ctx, items, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, item := range items.Items {
		if item.Spec.SourceSecretRef.Name == object.GetName() || item.Spec.KubeconfigSecretRef.Name == object.GetName() {
			requests = append(requests, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: item.Namespace, Name: item.Name}})
		}
	}

	return sortRequests(requests)
}
