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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/omniapi"
)

const (
	omniKubeconfigExportFinalizer = "omni.texashpc.com/kubeconfig-export-finalizer"
	kubeconfigSecretKey           = "kubeconfig"
	kubeconfigOwnerLabel          = "omni.texashpc.com/kubeconfig-export"
	kubeconfigClusterLabel        = "omni.texashpc.com/cluster"
	kubeconfigHashAnnotation      = "omni.texashpc.com/kubeconfig-hash"
	kubeconfigLastRotationAnno    = "omni.texashpc.com/kubeconfig-last-rotation"
	kubeconfigExpirationAnno      = "omni.texashpc.com/kubeconfig-expiration"
)

// OmniKubeconfigExportReconciler reconciles an OmniKubeconfigExport object.
type OmniKubeconfigExportReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Omni   omniapi.Client
}

// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnikubeconfigexports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnikubeconfigexports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnikubeconfigexports/finalizers,verbs=update
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
func (r *OmniKubeconfigExportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	export := &omniv1alpha1.OmniKubeconfigExport{}
	if err := r.Get(ctx, req.NamespacedName, export); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if !export.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, export)
	}

	if controllerutil.AddFinalizer(export, omniKubeconfigExportFinalizer) {
		if err := r.Update(ctx, export); err != nil {
			return ctrl.Result{}, err
		}
	}

	cluster := &omniv1alpha1.OmniCluster{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: export.Namespace, Name: export.Spec.ClusterRef.Name}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			if statusErr := r.updateStatus(ctx, export, nil, nil, omniv1alpha1.ReasonMissingCluster, fmt.Sprintf("OmniCluster %q does not exist", export.Spec.ClusterRef.Name), nil); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	connection := &omniv1alpha1.OmniConnection{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: export.Namespace, Name: cluster.Spec.ConnectionRef.Name}, connection); err != nil {
		if apierrors.IsNotFound(err) {
			if statusErr := r.updateStatus(ctx, export, nil, nil, omniv1alpha1.ReasonMissingConnection, fmt.Sprintf("OmniConnection %q does not exist", cluster.Spec.ConnectionRef.Name), nil); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: export.Namespace, Name: export.Spec.TargetSecretRef.Name}
	getSecretErr := r.Get(ctx, secretKey, secret)
	if getSecretErr != nil && !apierrors.IsNotFound(getSecretErr) {
		return ctrl.Result{}, getSecretErr
	}
	if apierrors.IsNotFound(getSecretErr) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: export.Namespace,
				Name:      export.Spec.TargetSecretRef.Name,
			},
			Type: corev1.SecretTypeOpaque,
		}
	}

	now := metav1.Now()
	rotationDue := getSecretErr != nil || export.Status.ObservedGeneration != export.Generation
	if !rotationDue && export.Status.ExpirationTime != nil {
		rotateAt := export.Status.ExpirationTime.Time
		if export.Spec.RenewBefore != nil {
			rotateAt = rotateAt.Add(-export.Spec.RenewBefore.Duration)
		}
		rotationDue = !now.Time.Before(rotateAt)
	}
	if !rotationDue && export.Status.KubeconfigHash == "" {
		rotationDue = true
	}

	hash := export.Status.KubeconfigHash
	lastRotation := export.Status.LastRotationTime
	expiration := export.Status.ExpirationTime

	if rotationDue {
		kubeconfig, err := r.omniClient().ServiceAccountKubeconfig(
			ctx,
			connection,
			clusterName(cluster),
			export.Spec.TTL.Duration,
			export.Spec.ServiceAccount.User,
			export.Spec.ServiceAccount.Groups,
		)
		if err != nil {
			if statusErr := r.updateStatus(ctx, export, lastRotation, expiration, omniv1alpha1.ReasonSyncFailed, err.Error(), nil); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			return ctrl.Result{}, err
		}

		sum := sha256.Sum256(kubeconfig)
		hash = hex.EncodeToString(sum[:])
		lastRotation = &now
		exp := metav1.NewTime(now.Add(export.Spec.TTL.Duration))
		expiration = &exp

		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[kubeconfigSecretKey] = kubeconfig
	}

	secret.Type = corev1.SecretTypeOpaque
	secret.Labels = mergeStringMaps(secret.Labels, map[string]string{
		kubeconfigOwnerLabel:   export.Name,
		kubeconfigClusterLabel: export.Spec.ClusterRef.Name,
	})
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[kubeconfigHashAnnotation] = hash
	if lastRotation != nil {
		secret.Annotations[kubeconfigLastRotationAnno] = lastRotation.Time.UTC().Format(time.RFC3339)
	}
	if expiration != nil {
		secret.Annotations[kubeconfigExpirationAnno] = expiration.Time.UTC().Format(time.RFC3339)
	}

	if export.Spec.DeletionPolicy == omniv1alpha1.KubeconfigExportDeletionPolicyOrphan {
		secret.OwnerReferences = removeOwnerReference(secret.OwnerReferences, export.GetUID())
	} else if err := controllerutil.SetControllerReference(export, secret, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(getSecretErr) {
		if err := r.Create(ctx, secret); err != nil {
			return ctrl.Result{}, err
		}
	} else if err := r.Update(ctx, secret); err != nil {
		return ctrl.Result{}, err
	}

	if statusErr := r.updateStatus(ctx, export, lastRotation, expiration, omniv1alpha1.ReasonSynced, fmt.Sprintf("exported kubeconfig to Secret %q", export.Spec.TargetSecretRef.Name), &hash); statusErr != nil {
		return ctrl.Result{}, statusErr
	}

	if export.Spec.RenewBefore != nil && expiration != nil {
		requeueAfter := expiration.Time.Add(-export.Spec.RenewBefore.Duration).Sub(time.Now())
		if requeueAfter < 0 {
			requeueAfter = 0
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

func (r *OmniKubeconfigExportReconciler) reconcileDelete(ctx context.Context, export *omniv1alpha1.OmniKubeconfigExport) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(export, omniKubeconfigExportFinalizer) && export.Spec.DeletionPolicy == omniv1alpha1.KubeconfigExportDeletionPolicyDelete {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: export.Namespace,
				Name:      export.Spec.TargetSecretRef.Name,
			},
		}
		if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	if controllerutil.RemoveFinalizer(export, omniKubeconfigExportFinalizer) {
		if err := r.Update(ctx, export); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *OmniKubeconfigExportReconciler) updateStatus(
	ctx context.Context,
	export *omniv1alpha1.OmniKubeconfigExport,
	lastRotation *metav1.Time,
	expiration *metav1.Time,
	reason string,
	message string,
	hash *string,
) error {
	key := client.ObjectKeyFromObject(export)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniKubeconfigExport{}
		if err := r.Get(ctx, key, latest); err != nil {
			return err
		}

		originalStatus := latest.Status.DeepCopy()
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.ClusterRef = latest.Spec.ClusterRef.Name
		latest.Status.TargetSecretRef = latest.Spec.TargetSecretRef.Name
		latest.Status.LastRotationTime = lastRotation
		latest.Status.ExpirationTime = expiration
		if hash != nil {
			latest.Status.KubeconfigHash = *hash
		}

		accepted := metav1.ConditionTrue
		if reason == omniv1alpha1.ReasonMissingCluster {
			accepted = metav1.ConditionFalse
		}
		omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionAccepted, accepted, latest.Generation, reason, message))

		readyStatus := metav1.ConditionTrue
		if reason == omniv1alpha1.ReasonMissingCluster || reason == omniv1alpha1.ReasonMissingConnection || reason == omniv1alpha1.ReasonSyncFailed {
			readyStatus = metav1.ConditionFalse
		}
		omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionSynced, readyStatus, latest.Generation, reason, message))
		omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, readyStatus, latest.Generation, reason, message))
		if reflect.DeepEqual(originalStatus, &latest.Status) {
			return nil
		}

		return r.Status().Update(ctx, latest)
	})
}

func (r *OmniKubeconfigExportReconciler) omniClient() omniapi.Client {
	if r.Omni != nil {
		return r.Omni
	}

	return &omniapi.RealClient{K8sClient: r.Client}
}

func clusterName(cluster *omniv1alpha1.OmniCluster) string {
	if cluster.Spec.ClusterName != "" {
		return cluster.Spec.ClusterName
	}

	return cluster.Name
}

func removeOwnerReference(references []metav1.OwnerReference, uid types.UID) []metav1.OwnerReference {
	if len(references) == 0 {
		return nil
	}

	updated := make([]metav1.OwnerReference, 0, len(references))
	for _, ref := range references {
		if ref.UID != uid {
			updated = append(updated, ref)
		}
	}

	return updated
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniKubeconfigExportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniKubeconfigExport{}, builder.WithPredicates(specOrDeletionChangedPredicate())).
		Owns(&corev1.Secret{}).
		Watches(&omniv1alpha1.OmniCluster{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return kubeconfigExportRequestsForCluster(ctx, r.Client, object)
		}), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Named("omnikubeconfigexport").
		Complete(r)
}
