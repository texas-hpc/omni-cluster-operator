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
	"strings"
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/cilium"
	"github.com/texas-hpc/omni-cluster-operator/internal/omniapi"
	"github.com/texas-hpc/omni-cluster-operator/internal/omnitemplate"
)

var errCiliumRenderPending = errors.New("OmniCilium render is pending")

// OmniClusterReconciler reconciles a OmniCluster object
type OmniClusterReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	SecretReader client.Reader
	Omni         omniapi.Client
}

// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnicontrolplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniworkers,verbs=get;list;watch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omnimachines,verbs=get;list;watch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniciliums,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *OmniClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cluster := &omniv1alpha1.OmniCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if !cluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
	}

	if !controllerutil.ContainsFinalizer(cluster, omniv1alpha1.Finalizer) {
		controllerutil.AddFinalizer(cluster, omniv1alpha1.Finalizer)
		if err := r.Update(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	requeueAfter := syncInterval(cluster)

	if cluster.Spec.Suspend {
		err := r.updateClusterStatus(ctx, cluster, func(status *omniv1alpha1.OmniClusterStatus) {
			status.ObservedGeneration = cluster.Generation
			omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, cluster.Generation, omniv1alpha1.ReasonSuspended, "remote Omni sync is suspended"))
		})

		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	connection := &omniv1alpha1.OmniConnection{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.Spec.ConnectionRef.Name}, connection); err != nil {
		statusErr := r.markClusterFailed(ctx, cluster, omniv1alpha1.ConditionSynced, omniv1alpha1.ReasonMissingConnection, fmt.Sprintf("failed to get OmniConnection %q: %v", cluster.Spec.ConnectionRef.Name, err))
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	inputs, err := r.templateInputs(ctx, cluster)
	if err != nil {
		if errors.Is(err, errCiliumRenderPending) {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}

		statusErr := r.markClusterFailed(ctx, cluster, omniv1alpha1.ConditionValidated, omniv1alpha1.ReasonMissingTemplate, err.Error())
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	rendered, err := omnitemplate.Render(inputs)
	if err != nil {
		statusErr := r.markClusterFailed(ctx, cluster, omniv1alpha1.ConditionValidated, omniv1alpha1.ReasonValidationFailed, err.Error())
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	if err := omnitemplate.Validate(rendered.Template, cluster.Spec.TemplateRoot); err != nil {
		statusErr := r.updateClusterStatus(ctx, cluster, func(status *omniv1alpha1.OmniClusterStatus) {
			setRenderedStatus(status, cluster, connection, rendered)
			omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionValidated, metav1.ConditionFalse, cluster.Generation, omniv1alpha1.ReasonValidationFailed, err.Error()))
			omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, cluster.Generation, omniv1alpha1.ReasonValidationFailed, err.Error()))
		})
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	now := metav1.Now()
	syncOutput, err := r.omniClient().SyncTemplate(ctx, connection, rendered.Template, cluster.Spec.TemplateRoot, omniapi.SyncOptions{
		DestroyMachines: false,
	})
	if err != nil {
		message := fmt.Sprintf("sync failed: %v", err)
		if strings.TrimSpace(syncOutput) != "" {
			message = fmt.Sprintf("%s\n%s", message, trimStatus(syncOutput))
		}

		statusErr := r.updateClusterStatus(ctx, cluster, func(status *omniv1alpha1.OmniClusterStatus) {
			setRenderedStatus(status, cluster, connection, rendered)
			status.LastAttemptTime = &now
			status.LastSyncOutput = trimStatus(syncOutput)
			omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionValidated, metav1.ConditionTrue, cluster.Generation, omniv1alpha1.ReasonValidated, "rendered Omni cluster template is valid"))
			omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionSynced, metav1.ConditionFalse, cluster.Generation, omniv1alpha1.ReasonSyncFailed, message))
			omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, cluster.Generation, omniv1alpha1.ReasonSyncFailed, message))
		})
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	statusOutput, statusErr := r.omniClient().StatusCluster(ctx, connection, rendered.ClusterName)
	err = r.updateClusterStatus(ctx, cluster, func(status *omniv1alpha1.OmniClusterStatus) {
		setRenderedStatus(status, cluster, connection, rendered)
		status.LastAttemptTime = &now
		status.LastSyncTime = &now
		status.LastSyncOutput = trimStatus(syncOutput)
		status.LastStatusOutput = trimStatus(statusOutput)
		omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionValidated, metav1.ConditionTrue, cluster.Generation, omniv1alpha1.ReasonValidated, "rendered Omni cluster template is valid"))
		omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionSynced, metav1.ConditionTrue, cluster.Generation, omniv1alpha1.ReasonSynced, "template synced to Omni"))

		if statusErr != nil {
			omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, cluster.Generation, omniv1alpha1.ReasonStatusFailed, statusErr.Error()))
			return
		}

		omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionTrue, cluster.Generation, omniv1alpha1.ReasonSynced, "cluster template is synced and status was read from Omni"))
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if statusErr != nil {
		log.Error(statusErr, "failed to read Omni cluster status", "cluster", rendered.ClusterName)
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *OmniClusterReconciler) reconcileDelete(ctx context.Context, cluster *omniv1alpha1.OmniCluster) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(cluster, omniv1alpha1.Finalizer) {
		return ctrl.Result{}, nil
	}

	if !cluster.Spec.DeletePolicy.Orphan {
		connection := &omniv1alpha1.OmniConnection{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.Spec.ConnectionRef.Name}, connection); err != nil {
			statusErr := r.markClusterFailed(ctx, cluster, omniv1alpha1.ConditionReady, omniv1alpha1.ReasonDeleteFailed, fmt.Sprintf("cannot delete remote Omni cluster without OmniConnection %q: %v", cluster.Spec.ConnectionRef.Name, err))
			if statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			return ctrl.Result{}, err
		}

		output, err := r.omniClient().DeleteCluster(ctx, connection, omnitemplate.ClusterName(cluster), omniapi.SyncOptions{
			DestroyMachines: cluster.Spec.DeletePolicy.DestroyMachines,
		})
		if err != nil {
			statusErr := r.updateClusterStatus(ctx, cluster, func(status *omniv1alpha1.OmniClusterStatus) {
				status.LastSyncOutput = trimStatus(output)
				omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, cluster.Generation, omniv1alpha1.ReasonDeleteFailed, err.Error()))
			})
			if statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(cluster, omniv1alpha1.Finalizer)
	if err := r.Update(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OmniClusterReconciler) templateInputs(ctx context.Context, cluster *omniv1alpha1.OmniCluster) (omnitemplate.Inputs, error) {
	controlPlanes := &omniv1alpha1.OmniControlPlaneList{}
	if err := r.List(ctx, controlPlanes, client.InNamespace(cluster.Namespace)); err != nil {
		return omnitemplate.Inputs{}, err
	}

	var selectedControlPlanes []omniv1alpha1.OmniControlPlane
	for _, controlPlane := range controlPlanes.Items {
		if controlPlane.Spec.ClusterRef.Name == cluster.Name {
			selectedControlPlanes = append(selectedControlPlanes, controlPlane)
		}
	}
	if len(selectedControlPlanes) != 1 {
		return omnitemplate.Inputs{}, fmt.Errorf("expected exactly one OmniControlPlane referencing %s/%s, found %d", cluster.Namespace, cluster.Name, len(selectedControlPlanes))
	}

	workersList := &omniv1alpha1.OmniWorkersList{}
	if err := r.List(ctx, workersList, client.InNamespace(cluster.Namespace)); err != nil {
		return omnitemplate.Inputs{}, err
	}

	var workers []omniv1alpha1.OmniWorkers
	for _, item := range workersList.Items {
		if item.Spec.ClusterRef.Name == cluster.Name {
			workers = append(workers, item)
		}
	}

	machineList := &omniv1alpha1.OmniMachineList{}
	if err := r.List(ctx, machineList, client.InNamespace(cluster.Namespace)); err != nil {
		return omnitemplate.Inputs{}, err
	}

	var machines []omniv1alpha1.OmniMachine
	for _, item := range machineList.Items {
		if item.Spec.ClusterRef.Name == cluster.Name {
			machines = append(machines, item)
		}
	}

	ciliumInput, err := r.ciliumInput(ctx, cluster)
	if err != nil {
		return omnitemplate.Inputs{}, err
	}

	return omnitemplate.Inputs{
		Cluster:      cluster,
		ControlPlane: &selectedControlPlanes[0],
		Workers:      workers,
		Machines:     machines,
		Cilium:       ciliumInput,
	}, nil
}

func (r *OmniClusterReconciler) ciliumInput(ctx context.Context, cluster *omniv1alpha1.OmniCluster) (*omnitemplate.CiliumInput, error) {
	ciliumList := &omniv1alpha1.OmniCiliumList{}
	if err := r.List(ctx, ciliumList, client.InNamespace(cluster.Namespace)); err != nil {
		return nil, err
	}

	var selected []omniv1alpha1.OmniCilium
	for _, item := range ciliumList.Items {
		if item.Spec.ClusterRef.Name == cluster.Name {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		return nil, nil
	}
	if len(selected) > 1 {
		return nil, fmt.Errorf("expected at most one OmniCilium referencing %s/%s, found %d", cluster.Namespace, cluster.Name, len(selected))
	}

	install := selected[0]
	specHash, err := cilium.SpecHash(&install)
	if err != nil {
		return nil, fmt.Errorf("OmniCilium %q is invalid: %w", install.Name, err)
	}

	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: install.Namespace, Name: cilium.RenderedManifestSecretName(&install)}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: OmniCilium %q has no rendered manifest Secret %q", errCiliumRenderPending, install.Name, secretKey.Name)
		}

		return nil, fmt.Errorf("OmniCilium %q has no rendered manifest Secret %q: %w", install.Name, secretKey.Name, err)
	}
	if !cilium.SecretHasCurrentManifest(secret, secret.Data, specHash) {
		return nil, fmt.Errorf("%w: OmniCilium %q rendered manifest Secret %q is not current", errCiliumRenderPending, install.Name, secretKey.Name)
	}

	objects, err := cilium.ParseRenderedManifest(secret.Data[cilium.RenderedManifestSecretKey])
	if err != nil {
		return nil, fmt.Errorf("parse OmniCilium %q rendered manifest: %w", install.Name, err)
	}

	_, kubeProxyReplacement, err := cilium.Values(&install)
	if err != nil {
		return nil, fmt.Errorf("OmniCilium %q is invalid: %w", install.Name, err)
	}

	return &omnitemplate.CiliumInput{
		ResourceName:         install.Name,
		ManifestName:         cilium.ManifestName(&install),
		Mode:                 cilium.Mode(&install),
		Manifest:             objects,
		KubeProxyReplacement: kubeProxyReplacement,
	}, nil
}

func (r *OmniClusterReconciler) omniClient() omniapi.Client {
	if r.Omni != nil {
		return r.Omni
	}

	secretReader := r.SecretReader
	if secretReader == nil {
		secretReader = r.Client
	}

	return &omniapi.RealClient{K8sClient: secretReader}
}

func (r *OmniClusterReconciler) markClusterFailed(ctx context.Context, cluster *omniv1alpha1.OmniCluster, conditionType, reason, message string) error {
	return r.updateClusterStatus(ctx, cluster, func(status *omniv1alpha1.OmniClusterStatus) {
		status.ObservedGeneration = cluster.Generation
		status.ClusterName = omnitemplate.ClusterName(cluster)
		omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(conditionType, metav1.ConditionFalse, cluster.Generation, reason, message))
		omniv1alpha1.SetCondition(&status.Conditions, omniv1alpha1.NewCondition(omniv1alpha1.ConditionReady, metav1.ConditionFalse, cluster.Generation, reason, message))
	})
}

func (r *OmniClusterReconciler) updateClusterStatus(ctx context.Context, cluster *omniv1alpha1.OmniCluster, mutate func(*omniv1alpha1.OmniClusterStatus)) error {
	key := client.ObjectKeyFromObject(cluster)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &omniv1alpha1.OmniCluster{}
		if err := r.Get(ctx, key, latest); err != nil {
			return err
		}

		mutate(&latest.Status)

		return r.Status().Update(ctx, latest)
	})
}

func setRenderedStatus(status *omniv1alpha1.OmniClusterStatus, cluster *omniv1alpha1.OmniCluster, connection *omniv1alpha1.OmniConnection, rendered *omnitemplate.Result) {
	status.ObservedGeneration = cluster.Generation
	status.ConnectionRef = connection.Name
	status.Endpoint = connection.Spec.Endpoint
	status.ClusterName = rendered.ClusterName
	status.TemplateHash = rendered.TemplateHash
	status.ControlPlaneRef = rendered.ControlPlaneRef
	status.WorkersRefs = rendered.WorkersRefs
	status.MachineRefs = rendered.MachineRefs
	status.CiliumRef = rendered.CiliumRef
}

func syncInterval(cluster *omniv1alpha1.OmniCluster) time.Duration {
	if cluster.Spec.SyncInterval.Duration > 0 {
		return cluster.Spec.SyncInterval.Duration
	}

	return 5 * time.Minute
}

func trimStatus(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= 4096 {
		return output
	}

	return output[:4096]
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniCluster{}, builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniConnection{}, handler.EnqueueRequestsFromMapFunc(r.clustersForConnection), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniControlPlane{}, handler.EnqueueRequestsFromMapFunc(requestForChildCluster), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniWorkers{}, handler.EnqueueRequestsFromMapFunc(requestForChildCluster), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniMachine{}, handler.EnqueueRequestsFromMapFunc(requestForChildCluster), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&omniv1alpha1.OmniCilium{}, handler.EnqueueRequestsFromMapFunc(requestForChildCluster), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.clustersForCiliumSecret)).
		Named("omnicluster").
		Complete(r)
}

func requestForChildCluster(_ context.Context, object client.Object) []reconcile.Request {
	switch typed := object.(type) {
	case *omniv1alpha1.OmniControlPlane:
		return requestForClusterRef(object.GetNamespace(), typed.Spec.ClusterRef.Name)
	case *omniv1alpha1.OmniWorkers:
		return requestForClusterRef(object.GetNamespace(), typed.Spec.ClusterRef.Name)
	case *omniv1alpha1.OmniMachine:
		return requestForClusterRef(object.GetNamespace(), typed.Spec.ClusterRef.Name)
	case *omniv1alpha1.OmniCilium:
		return requestForClusterRef(object.GetNamespace(), typed.Spec.ClusterRef.Name)
	default:
		return nil
	}
}

func requestForClusterRef(namespace, name string) []reconcile.Request {
	if name == "" {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{Namespace: namespace, Name: name},
	}}
}

func (r *OmniClusterReconciler) clustersForConnection(ctx context.Context, object client.Object) []reconcile.Request {
	clusterList := &omniv1alpha1.OmniClusterList{}
	if err := r.List(ctx, clusterList, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, cluster := range clusterList.Items {
		if cluster.Spec.ConnectionRef.Name == object.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.Name}})
		}
	}

	return sortRequests(requests)
}

func (r *OmniClusterReconciler) clustersForCiliumSecret(ctx context.Context, object client.Object) []reconcile.Request {
	if object.GetLabels()[cilium.RenderedManifestOwnerLabel] == "" {
		return nil
	}

	ciliumList := &omniv1alpha1.OmniCiliumList{}
	if err := r.List(ctx, ciliumList, client.InNamespace(object.GetNamespace())); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, install := range ciliumList.Items {
		if cilium.RenderedManifestSecretName(&install) == object.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: install.Namespace, Name: install.Spec.ClusterRef.Name}})
		}
	}

	return sortRequests(requests)
}
