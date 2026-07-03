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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/kubeconfigexport"
	"github.com/texas-hpc/omni-cluster-operator/internal/omniapi"
	"github.com/texas-hpc/omni-cluster-operator/internal/omnitemplate"
)

const kubeconfigExportRetryInterval = time.Minute

var errKubeconfigNotUsable = errors.New("kubeconfig is not usable")

type KubeconfigValidator interface {
	Validate(context.Context, []byte) error
}

type KubeconfigValidatorFunc func(context.Context, []byte) error

func (f KubeconfigValidatorFunc) Validate(ctx context.Context, kubeconfig []byte) error {
	return f(ctx, kubeconfig)
}

// OmniKubeconfigExportReconciler reconciles an OmniKubeconfigExport object.
type OmniKubeconfigExportReconciler struct {
	client.Client
	SecretReader        client.Reader
	Omni                omniapi.Client
	Clock               func() time.Time
	KubeconfigValidator KubeconfigValidator
}

// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnikubeconfigexports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnikubeconfigexports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnikubeconfigexports/finalizers,verbs=update
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *OmniKubeconfigExportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	item := &omniv1alpha1.OmniKubeconfigExport{}
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

	if err := kubeconfigExportSpecError(item); err != nil {
		statusErr := updateKubeconfigExportStatus(ctx, r.Client, item, kubeconfigExportStatusUpdate{
			exportErr: err,
			reason:    omniv1alpha1.ReasonValidationFailed,
			message:   err.Error(),
		})

		return ctrl.Result{}, statusErr
	}

	cluster, result, done, err := r.exportCluster(ctx, item)
	if done || err != nil {
		return result, err
	}

	connection, result, done, err := r.exportConnection(ctx, item, cluster)
	if done || err != nil {
		return result, err
	}

	clusterName := omnitemplate.ClusterName(cluster)
	specHash, err := kubeconfigexport.SpecHash(item, clusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	now := r.now()
	targetKey := kubeconfigexport.TargetSecretKey(item)
	if err := r.deletePreviousTargetSecret(ctx, item); err != nil {
		return ctrl.Result{}, err
	}

	secret, exists, err := r.targetSecret(ctx, item)
	if err != nil {
		return ctrl.Result{}, err
	}

	if exists {
		if state, current := r.reusableCurrentKubeconfigSecretState(ctx, secret, targetKey, specHash, now, item.Spec.RenewBefore); current {
			statusErr := updateKubeconfigExportStatus(ctx, r.Client, item, kubeconfigExportStatusUpdate{
				cluster:          cluster,
				connection:       connection,
				clusterExists:    true,
				acceptedKnown:    true,
				exported:         true,
				hash:             state.hash,
				expirationTime:   state.expirationTime,
				lastRotationTime: state.lastRotationTime,
				nextRotationTime: state.nextRotationTime,
			})
			if statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			return ctrl.Result{RequeueAfter: requeueAfter(now, state.nextRotationTime)}, nil
		}
	}

	kubeconfig, result, done, err := r.generateKubeconfig(ctx, item, cluster, connection, clusterName)
	if done || err != nil {
		return result, err
	}

	rotationTime := metav1.NewTime(now)
	expirationTime := metav1.NewTime(now.Add(item.Spec.TTL.Duration))
	nextRotationTime := kubeconfigexport.NextRotationTime(expirationTime, item.Spec.RenewBefore)
	kubeconfigHash := kubeconfigexport.Hash(kubeconfig)

	if !exists {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: kubeconfigexport.TargetSecretNamespace(item),
				Name:      item.Spec.TargetSecretRef.Name,
			},
			Type: corev1.SecretTypeOpaque,
		}
	}

	secret.Labels = mergeStringMaps(secret.Labels, kubeconfigexport.SecretLabels(item, clusterName))
	secret.Annotations = mergeStringMaps(secret.Annotations, kubeconfigexport.SecretAnnotations(item, specHash, kubeconfigHash, expirationTime, rotationTime))
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	if kubeconfigexport.IsOwnedSecret(item, secret) && item.Status.TargetSecretRef == item.Spec.TargetSecretRef.Name && item.Status.TargetSecretKey != "" && item.Status.TargetSecretKey != targetKey {
		delete(secret.Data, item.Status.TargetSecretKey)
	}
	secret.Data[targetKey] = kubeconfig

	if exists {
		if err := r.Update(ctx, secret); err != nil {
			return ctrl.Result{}, err
		}
	} else if err := r.Create(ctx, secret); err != nil {
		return ctrl.Result{}, err
	}

	logf.FromContext(ctx).V(1).Info("exported Omni workload kubeconfig", "secret", types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}.String(), "cluster", clusterName, "hash", kubeconfigHash)

	statusErr := updateKubeconfigExportStatus(ctx, r.Client, item, kubeconfigExportStatusUpdate{
		cluster:          cluster,
		connection:       connection,
		clusterExists:    true,
		acceptedKnown:    true,
		exported:         true,
		hash:             kubeconfigHash,
		expirationTime:   &expirationTime,
		lastRotationTime: &rotationTime,
		nextRotationTime: &nextRotationTime,
	})
	if statusErr != nil {
		return ctrl.Result{}, statusErr
	}

	return ctrl.Result{RequeueAfter: requeueAfter(now, &nextRotationTime)}, nil
}

func (r *OmniKubeconfigExportReconciler) reconcileDelete(ctx context.Context, item *omniv1alpha1.OmniKubeconfigExport) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(item, omniv1alpha1.Finalizer) {
		return ctrl.Result{}, nil
	}

	if item.Spec.DeletionPolicy == omniv1alpha1.KubeconfigExportDeletionPolicyDelete && item.Spec.TargetSecretRef.Name != "" {
		secret, exists, err := r.targetSecret(ctx, item)
		if err != nil {
			return ctrl.Result{}, err
		}
		if exists && kubeconfigexport.IsOwnedSecret(item, secret) {
			if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
	}

	controllerutil.RemoveFinalizer(item, omniv1alpha1.Finalizer)

	return ctrl.Result{}, r.Update(ctx, item)
}

func (r *OmniKubeconfigExportReconciler) ensureFinalizer(ctx context.Context, item *omniv1alpha1.OmniKubeconfigExport) (ctrl.Result, bool, error) {
	if controllerutil.ContainsFinalizer(item, omniv1alpha1.Finalizer) {
		return ctrl.Result{}, false, nil
	}

	controllerutil.AddFinalizer(item, omniv1alpha1.Finalizer)
	if err := r.Update(ctx, item); err != nil {
		return ctrl.Result{}, true, err
	}

	return ctrl.Result{Requeue: true}, true, nil
}

func (r *OmniKubeconfigExportReconciler) exportCluster(ctx context.Context, item *omniv1alpha1.OmniKubeconfigExport) (*omniv1alpha1.OmniCluster, ctrl.Result, bool, error) {
	cluster := &omniv1alpha1.OmniCluster{}
	clusterKey := client.ObjectKey{Namespace: item.Namespace, Name: item.Spec.ClusterRef.Name}
	if err := r.Get(ctx, clusterKey, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			statusErr := updateKubeconfigExportStatus(ctx, r.Client, item, kubeconfigExportStatusUpdate{
				clusterExists: false,
				acceptedKnown: true,
				reason:        omniv1alpha1.ReasonMissingCluster,
				message:       fmt.Sprintf("OmniCluster %q does not exist", item.Spec.ClusterRef.Name),
			})

			return nil, ctrl.Result{RequeueAfter: kubeconfigExportRetryInterval}, true, statusErr
		}

		return nil, ctrl.Result{}, true, err
	}

	return cluster, ctrl.Result{}, false, nil
}

func (r *OmniKubeconfigExportReconciler) exportConnection(ctx context.Context, item *omniv1alpha1.OmniKubeconfigExport, cluster *omniv1alpha1.OmniCluster) (*omniv1alpha1.OmniConnection, ctrl.Result, bool, error) {
	connection := &omniv1alpha1.OmniConnection{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: item.Namespace, Name: cluster.Spec.ConnectionRef.Name}, connection); err != nil {
		statusErr := updateKubeconfigExportStatus(ctx, r.Client, item, kubeconfigExportStatusUpdate{
			cluster:       cluster,
			clusterExists: true,
			acceptedKnown: true,
			exportErr:     err,
			reason:        omniv1alpha1.ReasonMissingConnection,
			message:       fmt.Sprintf("failed to get OmniConnection %q: %v", cluster.Spec.ConnectionRef.Name, err),
		})
		if statusErr != nil {
			return nil, ctrl.Result{}, true, statusErr
		}

		return nil, ctrl.Result{RequeueAfter: kubeconfigExportRetryInterval}, true, nil
	}

	return connection, ctrl.Result{}, false, nil
}

func (r *OmniKubeconfigExportReconciler) generateKubeconfig(ctx context.Context, item *omniv1alpha1.OmniKubeconfigExport, cluster *omniv1alpha1.OmniCluster, connection *omniv1alpha1.OmniConnection, clusterName string) ([]byte, ctrl.Result, bool, error) {
	kubeconfig, err := r.omniClient().Kubeconfig(ctx, connection, clusterName, omniapi.KubeconfigOptions{
		TTL:    item.Spec.TTL.Duration,
		User:   item.Spec.ServiceAccount.User,
		Groups: append([]string(nil), item.Spec.ServiceAccount.Groups...),
	})
	if err != nil {
		statusErr := updateKubeconfigExportStatus(ctx, r.Client, item, kubeconfigExportStatusUpdate{
			cluster:       cluster,
			connection:    connection,
			clusterExists: true,
			acceptedKnown: true,
			exportErr:     err,
			reason:        omniv1alpha1.ReasonExportFailed,
			message:       fmt.Sprintf("failed to export kubeconfig: %v", err),
		})
		if statusErr != nil {
			return nil, ctrl.Result{}, true, statusErr
		}

		return nil, ctrl.Result{RequeueAfter: kubeconfigExportRetryInterval}, true, err
	}
	loadedKubeconfig, err := clientcmd.Load(kubeconfig)
	if err != nil {
		exportErr := fmt.Errorf("generated kubeconfig is invalid: %w", err)
		statusErr := updateKubeconfigExportStatus(ctx, r.Client, item, kubeconfigExportStatusUpdate{
			cluster:       cluster,
			connection:    connection,
			clusterExists: true,
			acceptedKnown: true,
			exportErr:     exportErr,
			reason:        omniv1alpha1.ReasonExportFailed,
			message:       exportErr.Error(),
		})
		if statusErr != nil {
			return nil, ctrl.Result{}, true, statusErr
		}

		return nil, ctrl.Result{RequeueAfter: kubeconfigExportRetryInterval}, true, exportErr
	}
	if item.Spec.ContextNamespace != "" {
		contextName := loadedKubeconfig.CurrentContext
		kubeContext, ok := loadedKubeconfig.Contexts[contextName]
		if !ok {
			exportErr := fmt.Errorf("generated kubeconfig current context %q was not found", contextName)
			statusErr := updateKubeconfigExportStatus(ctx, r.Client, item, kubeconfigExportStatusUpdate{
				cluster:       cluster,
				connection:    connection,
				clusterExists: true,
				acceptedKnown: true,
				exportErr:     exportErr,
				reason:        omniv1alpha1.ReasonExportFailed,
				message:       exportErr.Error(),
			})
			if statusErr != nil {
				return nil, ctrl.Result{}, true, statusErr
			}

			return nil, ctrl.Result{RequeueAfter: kubeconfigExportRetryInterval}, true, exportErr
		}
		kubeContext.Namespace = item.Spec.ContextNamespace
		kubeconfig, err = clientcmd.Write(*loadedKubeconfig)
		if err != nil {
			exportErr := fmt.Errorf("write kubeconfig with context namespace: %w", err)
			statusErr := updateKubeconfigExportStatus(ctx, r.Client, item, kubeconfigExportStatusUpdate{
				cluster:       cluster,
				connection:    connection,
				clusterExists: true,
				acceptedKnown: true,
				exportErr:     exportErr,
				reason:        omniv1alpha1.ReasonExportFailed,
				message:       exportErr.Error(),
			})
			if statusErr != nil {
				return nil, ctrl.Result{}, true, statusErr
			}

			return nil, ctrl.Result{RequeueAfter: kubeconfigExportRetryInterval}, true, exportErr
		}
	}

	return kubeconfig, ctrl.Result{}, false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniKubeconfigExportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniKubeconfigExport{}, builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniCluster{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return kubeconfigExportRequestsForCluster(ctx, r.Client, object)
		}), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniConnection{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return r.kubeconfigExportRequestsForConnection(ctx, object)
		}), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(kubeconfigExportRequestsForSecret)).
		Named("omnikubeconfigexport").
		Complete(r)
}

func (r *OmniKubeconfigExportReconciler) omniClient() omniapi.Client {
	if r.Omni != nil {
		return r.Omni
	}

	secretReader := r.SecretReader
	if secretReader == nil {
		secretReader = r.Client
	}

	return &omniapi.RealClient{K8sClient: secretReader}
}

func (r *OmniKubeconfigExportReconciler) now() time.Time {
	if r.Clock != nil {
		return r.Clock().UTC()
	}

	return time.Now().UTC()
}

func (r *OmniKubeconfigExportReconciler) kubeconfigValidator() KubeconfigValidator {
	if r.KubeconfigValidator != nil {
		return r.KubeconfigValidator
	}

	return workloadKubeconfigValidator{}
}

type workloadKubeconfigValidator struct{}

func (workloadKubeconfigValidator) Validate(ctx context.Context, kubeconfig []byte) error {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("%w: %v", errKubeconfigNotUsable, err)
	}
	config.Timeout = 10 * time.Second

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("%w: %v", errKubeconfigNotUsable, err)
	}
	_, err = clientset.Discovery().ServerVersion()

	return err
}

func kubeconfigValidationRequiresRotation(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, errKubeconfigNotUsable) || apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err)
}

func (r *OmniKubeconfigExportReconciler) targetSecret(ctx context.Context, item *omniv1alpha1.OmniKubeconfigExport) (*corev1.Secret, bool, error) {
	secretReader := r.SecretReader
	if secretReader == nil {
		secretReader = r.Client
	}

	// Prefer observed status fields for target Secret lookup to handle spec changes correctly
	targetNamespace := kubeconfigexport.TargetSecretNamespace(item)
	targetName := item.Spec.TargetSecretRef.Name
	if item.Status.TargetSecretRef != "" && item.Status.TargetSecretNamespace != "" {
		targetNamespace = item.Status.TargetSecretNamespace
		targetName = item.Status.TargetSecretRef
	}

	secret := &corev1.Secret{}
	err := secretReader.Get(ctx, client.ObjectKey{Namespace: targetNamespace, Name: targetName}, secret)
	if err == nil {
		return secret, true, nil
	}
	if apierrors.IsNotFound(err) {
		return nil, false, nil
	}

	return nil, false, err
}

func (r *OmniKubeconfigExportReconciler) deletePreviousTargetSecret(ctx context.Context, item *omniv1alpha1.OmniKubeconfigExport) error {
	currentNamespace := kubeconfigexport.TargetSecretNamespace(item)
	previousNamespace := item.Status.TargetSecretNamespace
	if previousNamespace == "" {
		previousNamespace = item.Namespace
	}

	if item.Spec.DeletionPolicy != omniv1alpha1.KubeconfigExportDeletionPolicyDelete ||
		item.Status.TargetSecretRef == "" ||
		(item.Status.TargetSecretRef == item.Spec.TargetSecretRef.Name && previousNamespace == currentNamespace) {
		return nil
	}

	secretReader := r.SecretReader
	if secretReader == nil {
		secretReader = r.Client
	}

	secret := &corev1.Secret{}
	err := secretReader.Get(ctx, client.ObjectKey{Namespace: previousNamespace, Name: item.Status.TargetSecretRef}, secret)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !kubeconfigexport.IsOwnedSecret(item, secret) {
		return nil
	}

	return r.Delete(ctx, secret)
}

type currentKubeconfigState struct {
	hash             string
	expirationTime   *metav1.Time
	lastRotationTime *metav1.Time
	nextRotationTime *metav1.Time
}

func currentKubeconfigSecretState(secret *corev1.Secret, targetKey, specHash string, now time.Time, renewBefore *metav1.Duration) (*currentKubeconfigState, bool) {
	data := secret.Data[targetKey]
	if len(data) == 0 || secret.Annotations[kubeconfigexport.SpecHashAnnotation] != specHash {
		return nil, false
	}

	hash := kubeconfigexport.Hash(data)
	if secret.Annotations[kubeconfigexport.HashAnnotation] != hash {
		return nil, false
	}

	expirationTime, err := kubeconfigexport.AnnotationTime(secret, kubeconfigexport.ExpirationAnnotation)
	if err != nil || expirationTime == nil {
		return nil, false
	}
	if kubeconfigexport.RotationDue(now, *expirationTime, renewBefore) {
		return nil, false
	}

	lastRotationTime, err := kubeconfigexport.AnnotationTime(secret, kubeconfigexport.LastRotationAnnotation)
	if err != nil {
		return nil, false
	}

	nextRotationTime := kubeconfigexport.NextRotationTime(*expirationTime, renewBefore)

	return &currentKubeconfigState{
		hash:             hash,
		expirationTime:   expirationTime,
		lastRotationTime: lastRotationTime,
		nextRotationTime: &nextRotationTime,
	}, true
}

func (r *OmniKubeconfigExportReconciler) reusableCurrentKubeconfigSecretState(ctx context.Context, secret *corev1.Secret, targetKey, specHash string, now time.Time, renewBefore *metav1.Duration) (*currentKubeconfigState, bool) {
	state, current := currentKubeconfigSecretState(secret, targetKey, specHash, now, renewBefore)
	if !current {
		return nil, false
	}

	validationErr := r.kubeconfigValidator().Validate(ctx, secret.Data[targetKey])
	if kubeconfigValidationRequiresRotation(validationErr) {
		logf.FromContext(ctx).Info("existing exported kubeconfig is no longer usable; rotating", "secret", types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}.String(), "error", validationErr)

		return nil, false
	}
	if validationErr != nil {
		logf.FromContext(ctx).V(1).Info("existing exported kubeconfig validation failed without proving it stale; keeping current Secret", "secret", types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}.String(), "error", validationErr)
	}

	return state, true
}

func requeueAfter(now time.Time, nextRotationTime *metav1.Time) time.Duration {
	if nextRotationTime == nil {
		return 0
	}

	delay := nextRotationTime.Sub(now)
	if delay <= 0 {
		return 0
	}

	return delay
}

func kubeconfigExportSpecError(item *omniv1alpha1.OmniKubeconfigExport) error {
	if strings.TrimSpace(item.Spec.ClusterRef.Name) == "" {
		return fmt.Errorf("clusterRef.name is required")
	}
	if strings.TrimSpace(item.Spec.TargetSecretRef.Name) == "" {
		return fmt.Errorf("targetSecretRef.name is required")
	}
	if item.Spec.TargetSecretRef.Namespace != "" && strings.TrimSpace(item.Spec.TargetSecretRef.Namespace) == "" {
		return fmt.Errorf("targetSecretRef.namespace is required when set")
	}
	if strings.TrimSpace(kubeconfigexport.TargetSecretKey(item)) == "" {
		return fmt.Errorf("targetSecretRef.key is required")
	}
	if strings.TrimSpace(item.Spec.ServiceAccount.User) == "" {
		return fmt.Errorf("serviceAccount.user is required")
	}
	if len(item.Spec.ServiceAccount.Groups) == 0 {
		return fmt.Errorf("serviceAccount.groups requires at least one group")
	}
	for _, group := range item.Spec.ServiceAccount.Groups {
		group = strings.TrimSpace(group)
		if group == "" {
			return fmt.Errorf("serviceAccount.groups must not contain blank values")
		}
		if group == omniv1alpha1.KubeconfigClusterAdminGroup && !item.Spec.ServiceAccount.AllowClusterAdmin {
			return fmt.Errorf("system:masters requires serviceAccount.allowClusterAdmin: true")
		}
	}
	if item.Spec.TTL.Duration <= 0 {
		return fmt.Errorf("ttl must be greater than zero")
	}
	if item.Spec.RenewBefore != nil {
		if item.Spec.RenewBefore.Duration <= 0 {
			return fmt.Errorf("renewBefore must be greater than zero")
		}
		if item.Spec.RenewBefore.Duration >= item.Spec.TTL.Duration {
			return fmt.Errorf("renewBefore must be less than ttl")
		}
	}
	if item.Spec.DeletionPolicy != omniv1alpha1.KubeconfigExportDeletionPolicyDelete && item.Spec.DeletionPolicy != omniv1alpha1.KubeconfigExportDeletionPolicyOrphan {
		return fmt.Errorf("deletionPolicy must be Delete or Orphan")
	}

	return nil
}

type kubeconfigExportStatusUpdate struct {
	cluster          *omniv1alpha1.OmniCluster
	connection       *omniv1alpha1.OmniConnection
	clusterExists    bool
	acceptedKnown    bool
	exported         bool
	hash             string
	expirationTime   *metav1.Time
	lastRotationTime *metav1.Time
	nextRotationTime *metav1.Time
	exportErr        error
	reason           string
	message          string
}

func updateKubeconfigExportStatus(ctx context.Context, c client.Client, item *omniv1alpha1.OmniKubeconfigExport, input kubeconfigExportStatusUpdate) error {
	key := client.ObjectKeyFromObject(item)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniKubeconfigExport{}
		if err := c.Get(ctx, key, latest); err != nil {
			return err
		}

		originalStatus := latest.Status.DeepCopy()
		latest.Status.ObservedGeneration = latest.Generation
		latest.Status.ClusterRef = latest.Spec.ClusterRef.Name
		latest.Status.TargetSecretRef = latest.Spec.TargetSecretRef.Name
		latest.Status.TargetSecretNamespace = kubeconfigexport.TargetSecretNamespace(latest)
		latest.Status.TargetSecretKey = kubeconfigexport.TargetSecretKey(latest)
		latest.Status.ServiceAccountUser = latest.Spec.ServiceAccount.User
		latest.Status.ServiceAccountGroups = append([]string(nil), latest.Spec.ServiceAccount.Groups...)

		if input.cluster != nil {
			latest.Status.ClusterName = omnitemplate.ClusterName(input.cluster)
		}
		if input.connection != nil {
			latest.Status.ConnectionRef = input.connection.Name
			latest.Status.Endpoint = input.connection.Spec.Endpoint
		}
		latest.Status.KubeconfigHash = input.hash
		latest.Status.ExpirationTime = copyTime(input.expirationTime)
		latest.Status.LastRotationTime = copyTime(input.lastRotationTime)
		latest.Status.NextRotationTime = copyTime(input.nextRotationTime)

		if input.acceptedKnown {
			omniv1alpha1.SetCondition(&latest.Status.Conditions, acceptedCondition(latest.Generation, latest.Spec.ClusterRef.Name, input.clusterExists))
		}
		switch {
		case input.exported:
			target := client.ObjectKey{Namespace: kubeconfigexport.TargetSecretNamespace(latest), Name: latest.Spec.TargetSecretRef.Name}
			message := fmt.Sprintf("exported kubeconfig to Secret %q key %q", target.String(), kubeconfigexport.TargetSecretKey(latest))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionExported, metav1.ConditionTrue, latest.Generation, omniv1alpha1.ReasonExported, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionTrue, latest.Generation, omniv1alpha1.ReasonExported, message))
		case input.acceptedKnown && !input.clusterExists:
			message := input.message
			if message == "" {
				message = fmt.Sprintf("OmniCluster %q does not exist", latest.Spec.ClusterRef.Name)
			}
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionExported, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonMissingCluster, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, latest.Generation, omniv1alpha1.ReasonMissingCluster, message))
		default:
			reason := input.reason
			if reason == "" {
				reason = omniv1alpha1.ReasonExportFailed
			}
			message := input.message
			if message == "" && input.exportErr != nil {
				message = input.exportErr.Error()
			}
			if message == "" {
				message = "kubeconfig export failed"
			}
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionExported, metav1.ConditionFalse, latest.Generation, reason, message))
			omniv1alpha1.SetCondition(&latest.Status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, latest.Generation, reason, message))
		}

		if reflect.DeepEqual(originalStatus, &latest.Status) {
			return nil
		}

		return c.Status().Update(ctx, latest)
	})
}

func copyTime(value *metav1.Time) *metav1.Time {
	if value == nil {
		return nil
	}

	copied := *value

	return &copied
}

func kubeconfigExportRequestsForCluster(ctx context.Context, c client.Client, object client.Object) []reconcile.Request {
	items := &omniv1alpha1.OmniKubeconfigExportList{}
	if err := c.List(ctx, items, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, item := range items.Items {
		if item.Spec.ClusterRef.Name == object.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: item.Namespace, Name: item.Name}})
		}
	}

	return requests
}

func (r *OmniKubeconfigExportReconciler) kubeconfigExportRequestsForConnection(ctx context.Context, object client.Object) []reconcile.Request {
	clusters := &omniv1alpha1.OmniClusterList{}
	if err := r.List(ctx, clusters, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	clusterNames := map[string]struct{}{}
	for _, cluster := range clusters.Items {
		if cluster.Spec.ConnectionRef.Name == object.GetName() {
			clusterNames[cluster.Name] = struct{}{}
		}
	}
	if len(clusterNames) == 0 {
		return nil
	}

	items := &omniv1alpha1.OmniKubeconfigExportList{}
	if err := r.List(ctx, items, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, item := range items.Items {
		if _, ok := clusterNames[item.Spec.ClusterRef.Name]; ok {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: item.Namespace, Name: item.Name}})
		}
	}

	return requests
}

func kubeconfigExportRequestsForSecret(_ context.Context, object client.Object) []reconcile.Request {
	if object == nil || object.GetAnnotations() == nil {
		return nil
	}

	ownerName := object.GetAnnotations()[kubeconfigexport.OwnerAnnotation]
	if ownerName == "" {
		return nil
	}
	ownerNamespace := object.GetAnnotations()[kubeconfigexport.OwnerNamespaceAnnotation]
	if ownerNamespace == "" {
		ownerNamespace = object.GetNamespace()
	}

	return []reconcile.Request{{NamespacedName: client.ObjectKey{Namespace: ownerNamespace, Name: ownerName}}}
}
