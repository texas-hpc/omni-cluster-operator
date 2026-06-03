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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/helmrelease"
	"github.com/texas-hpc/omni-cluster-operator/internal/kubeconfigexport"
	"github.com/texas-hpc/omni-cluster-operator/internal/omniapi"
)

const (
	testClusterName      = "edge"
	testNamespace        = "default"
	testMachineName      = "node-1"
	testWorkersName      = "workers"
	testControlPlaneName = "edge-cp"
	testOtherClusterName = "other"
	testConnectionName   = "omni"
	testOtherConnection  = "other-omni"
	testOldKubeconfigKey = "old-kubeconfig"
	testHelmReleaseName  = "metrics-server"
	testHelmNamespace    = "kube-system"
	testHelmChartVersion = "3.13.0"
)

type fakeOmni struct {
	pingErr           error
	pingCalls         int
	syncErr           error
	statusErr         error
	syncedTemplate    []byte
	syncOptions       []omniapi.SyncOptions
	syncCalls         int
	deleteCalls       []string
	deleteOptions     []omniapi.SyncOptions
	kubeconfigErr     error
	kubeconfigData    []byte
	kubeconfigCalls   int
	kubeconfigCluster string
	kubeconfigOptions []omniapi.KubeconfigOptions
}

type fakeHelmReleaseClient struct {
	reconcileResult  *helmrelease.Result
	reconcileErr     error
	reconcileCalls   int
	reconcileConfigs [][]byte

	uninstallResult  *helmrelease.Result
	uninstallErr     error
	uninstallCalls   int
	uninstallConfigs [][]byte
}

func (f *fakeHelmReleaseClient) Reconcile(_ context.Context, _ *omniv1alpha1.OmniHelmRelease, kubeconfig []byte) (*helmrelease.Result, error) {
	f.reconcileCalls++
	f.reconcileConfigs = append(f.reconcileConfigs, append([]byte(nil), kubeconfig...))
	if f.reconcileResult == nil {
		f.reconcileResult = &helmrelease.Result{
			Action:       helmrelease.ActionInstall,
			ReleaseName:  testHelmReleaseName,
			Namespace:    testHelmNamespace,
			Chart:        testHelmReleaseName,
			ChartVersion: testHelmChartVersion,
			Revision:     1,
			Status:       helmrelease.StatusDeployed,
		}
	}

	return f.reconcileResult, f.reconcileErr
}

func (f *fakeHelmReleaseClient) Uninstall(_ context.Context, _ *omniv1alpha1.OmniHelmRelease, kubeconfig []byte) (*helmrelease.Result, error) {
	f.uninstallCalls++
	f.uninstallConfigs = append(f.uninstallConfigs, append([]byte(nil), kubeconfig...))
	if f.uninstallResult == nil {
		f.uninstallResult = &helmrelease.Result{
			Action:       helmrelease.ActionUninstall,
			ReleaseName:  testHelmReleaseName,
			Namespace:    testHelmNamespace,
			Chart:        testHelmReleaseName,
			ChartVersion: testHelmChartVersion,
			Revision:     2,
			Status:       helmrelease.StatusUninstalled,
		}
	}

	return f.uninstallResult, f.uninstallErr
}

func (f *fakeOmni) Ping(_ context.Context, connection *omniv1alpha1.OmniConnection) (string, error) {
	f.pingCalls++
	return fmt.Sprintf("connected to %s", connection.Spec.Endpoint), f.pingErr
}

func (f *fakeOmni) SyncTemplate(_ context.Context, _ *omniv1alpha1.OmniConnection, templateBytes []byte, _ string, options omniapi.SyncOptions) (string, error) {
	f.syncCalls++
	f.syncOptions = append(f.syncOptions, options)
	f.syncedTemplate = append([]byte(nil), templateBytes...)
	return "synced", f.syncErr
}

func (f *fakeOmni) DeleteCluster(_ context.Context, _ *omniv1alpha1.OmniConnection, clusterName string, options omniapi.SyncOptions) (string, error) {
	f.deleteCalls = append(f.deleteCalls, clusterName)
	f.deleteOptions = append(f.deleteOptions, options)
	return "deleted", nil
}

func (f *fakeOmni) StatusCluster(_ context.Context, _ *omniv1alpha1.OmniConnection, clusterName string) (string, error) {
	return fmt.Sprintf("status %s", clusterName), f.statusErr
}

func (f *fakeOmni) Kubeconfig(_ context.Context, _ *omniv1alpha1.OmniConnection, clusterName string, options omniapi.KubeconfigOptions) ([]byte, error) {
	f.kubeconfigCalls++
	f.kubeconfigCluster = clusterName
	f.kubeconfigOptions = append(f.kubeconfigOptions, options)
	if f.kubeconfigData == nil {
		f.kubeconfigData = testKubeconfigBytes("automation-token")
	}

	return append([]byte(nil), f.kubeconfigData...), f.kubeconfigErr
}

func TestOmniClusterDoesNotDestroyMachinesDuringNormalSync(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	omni := &fakeOmni{}
	cluster := testCluster()
	cluster.Spec.DeletePolicy.DestroyMachines = true
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniConnection{}, &omniv1alpha1.OmniControlPlane{}, &omniv1alpha1.OmniWorkers{}, &omniv1alpha1.OmniMachine{}).
		WithObjects(testConnection(), cluster, testControlPlane(), testWorkers()).
		Build()

	reconciler := &OmniClusterReconciler{Client: k8sClient, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testClusterName}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	if len(omni.syncOptions) != 1 {
		t.Fatalf("syncOptions length = %d, want 1", len(omni.syncOptions))
	}
	if omni.syncOptions[0].DestroyMachines {
		t.Fatal("DestroyMachines was passed to normal SyncTemplate")
	}
}

func TestOmniClusterReconcilesTemplateToOmni(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	omni := &fakeOmni{}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniConnection{}, &omniv1alpha1.OmniControlPlane{}, &omniv1alpha1.OmniWorkers{}, &omniv1alpha1.OmniMachine{}).
		WithObjects(testConnection(), testCluster(), testControlPlane(), testWorkers()).
		Build()

	reconciler := &OmniClusterReconciler{Client: k8sClient, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testClusterName}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	if omni.syncCalls != 1 {
		t.Fatalf("syncCalls = %d, want 1", omni.syncCalls)
	}
	if len(omni.syncedTemplate) == 0 {
		t.Fatal("expected rendered template to be synced")
	}

	cluster := &omniv1alpha1.OmniCluster{}
	if err := k8sClient.Get(ctx, request.NamespacedName, cluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}

	if got := meta.FindStatusCondition(cluster.Status.Conditions, omniv1alpha1.ConditionReady); got == nil || got.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition = %#v, want True", got)
	}
	if cluster.Status.TemplateHash == "" {
		t.Fatal("TemplateHash is empty")
	}
	if cluster.Status.ClusterName != testClusterName {
		t.Fatalf("ClusterName = %q, want edge", cluster.Status.ClusterName)
	}
}

func TestOmniClusterMissingControlPlaneDoesNotSync(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	omni := &fakeOmni{}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniCluster{}).
		WithObjects(testConnection(), testCluster()).
		Build()

	reconciler := &OmniClusterReconciler{Client: k8sClient, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testClusterName}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	if omni.syncCalls != 0 {
		t.Fatalf("syncCalls = %d, want 0", omni.syncCalls)
	}

	cluster := &omniv1alpha1.OmniCluster{}
	if err := k8sClient.Get(ctx, request.NamespacedName, cluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if got := meta.FindStatusCondition(cluster.Status.Conditions, omniv1alpha1.ConditionValidated); got == nil || got.Status != metav1.ConditionFalse {
		t.Fatalf("Validated condition = %#v, want False", got)
	}
}

func TestOmniClusterDeleteCallsOmniFinalizer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	omni := &fakeOmni{}
	deletionTime := metav1.Now()
	cluster := testCluster()
	cluster.Finalizers = []string{omniv1alpha1.Finalizer}
	cluster.DeletionTimestamp = &deletionTime
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniCluster{}).
		WithObjects(testConnection(), cluster).
		Build()

	reconciler := &OmniClusterReconciler{Client: k8sClient, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testClusterName}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(omni.deleteCalls) != 1 || omni.deleteCalls[0] != testClusterName {
		t.Fatalf("deleteCalls = %#v, want [edge]", omni.deleteCalls)
	}
}

func TestOmniClusterDeletePassesDestroyMachines(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	omni := &fakeOmni{}
	deletionTime := metav1.Now()
	cluster := testCluster()
	cluster.Spec.DeletePolicy.DestroyMachines = true
	cluster.Finalizers = []string{omniv1alpha1.Finalizer}
	cluster.DeletionTimestamp = &deletionTime
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniCluster{}).
		WithObjects(testConnection(), cluster).
		Build()

	reconciler := &OmniClusterReconciler{Client: k8sClient, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testClusterName}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(omni.deleteOptions) != 1 {
		t.Fatalf("deleteOptions length = %d, want 1", len(omni.deleteOptions))
	}
	if !omni.deleteOptions[0].DestroyMachines {
		t.Fatal("DestroyMachines was not passed to DeleteCluster")
	}
}

func TestOmniClusterDeleteRemovesLegacyFinalizer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	omni := &fakeOmni{}
	deletionTime := metav1.Now()
	cluster := testCluster()
	cluster.Finalizers = []string{omniv1alpha1.LegacyFinalizer}
	cluster.DeletionTimestamp = &deletionTime
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniCluster{}).
		WithObjects(testConnection(), cluster).
		Build()

	reconciler := &OmniClusterReconciler{Client: k8sClient, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testClusterName}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify the cluster was deleted from Omni
	if len(omni.deleteCalls) != 1 || omni.deleteCalls[0] != testClusterName {
		t.Fatalf("deleteCalls = %#v, want [edge]", omni.deleteCalls)
	}

	// Verify the cluster object is deleted (or finalizer was removed)
	updated := &omniv1alpha1.OmniCluster{}
	err := k8sClient.Get(ctx, request.NamespacedName, updated)
	if apierrors.IsNotFound(err) {
		// Expected: object was deleted when last finalizer was removed
		return
	}
	if err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	// If object still exists, verify finalizers are empty
	if len(updated.Finalizers) != 0 {
		t.Fatalf("finalizers = %#v, want empty (legacy finalizer should be removed)", updated.Finalizers)
	}
}

func TestOmniClusterDeleteRemovesBothFinalizers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	omni := &fakeOmni{}
	deletionTime := metav1.Now()
	cluster := testCluster()
	// Cluster has both finalizers (migration scenario)
	cluster.Finalizers = []string{omniv1alpha1.Finalizer, omniv1alpha1.LegacyFinalizer}
	cluster.DeletionTimestamp = &deletionTime
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniCluster{}).
		WithObjects(testConnection(), cluster).
		Build()

	reconciler := &OmniClusterReconciler{Client: k8sClient, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testClusterName}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify the cluster was deleted from Omni
	if len(omni.deleteCalls) != 1 || omni.deleteCalls[0] != testClusterName {
		t.Fatalf("deleteCalls = %#v, want [edge]", omni.deleteCalls)
	}

	// Verify the cluster object is deleted (or finalizers were removed)
	updated := &omniv1alpha1.OmniCluster{}
	err := k8sClient.Get(ctx, request.NamespacedName, updated)
	if apierrors.IsNotFound(err) {
		// Expected: object was deleted when last finalizer was removed
		return
	}
	if err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	// If object still exists, verify finalizers are empty
	if len(updated.Finalizers) != 0 {
		t.Fatalf("finalizers = %#v, want empty (both finalizers should be removed)", updated.Finalizers)
	}
}

func TestOmniClusterDoesNotAddFinalizerWhenLegacyFinalizerPresent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	omni := &fakeOmni{}
	cluster := testCluster()
	cluster.Finalizers = []string{omniv1alpha1.LegacyFinalizer}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniConnection{}, &omniv1alpha1.OmniControlPlane{}, &omniv1alpha1.OmniWorkers{}).
		WithObjects(testConnection(), cluster, testControlPlane(), testWorkers()).
		Build()

	reconciler := &OmniClusterReconciler{Client: k8sClient, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testClusterName}}

	// First reconcile should not add new finalizer since legacy is present
	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}

	// Verify only the legacy finalizer is present (no new finalizer added)
	updated := &omniv1alpha1.OmniCluster{}
	if err := k8sClient.Get(ctx, request.NamespacedName, updated); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if len(updated.Finalizers) != 1 || updated.Finalizers[0] != omniv1alpha1.LegacyFinalizer {
		t.Fatalf("finalizers = %#v, want only legacy finalizer", updated.Finalizers)
	}

	// Verify reconciliation proceeded normally (sync was called)
	if omni.syncCalls != 1 {
		t.Fatalf("syncCalls = %d, want 1 (reconcile should proceed with legacy finalizer)", omni.syncCalls)
	}
}

func TestOmniClusterAddsNewFinalizerWhenNoFinalizerPresent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	omni := &fakeOmni{}
	cluster := testCluster()
	// No finalizers initially
	cluster.Finalizers = nil
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniConnection{}, &omniv1alpha1.OmniControlPlane{}, &omniv1alpha1.OmniWorkers{}).
		WithObjects(testConnection(), cluster, testControlPlane(), testWorkers()).
		Build()

	reconciler := &OmniClusterReconciler{Client: k8sClient, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testClusterName}}

	// First reconcile should add the new finalizer
	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}

	// Verify the new finalizer was added
	updated := &omniv1alpha1.OmniCluster{}
	if err := k8sClient.Get(ctx, request.NamespacedName, updated); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if len(updated.Finalizers) != 1 || updated.Finalizers[0] != omniv1alpha1.Finalizer {
		t.Fatalf("finalizers = %#v, want new finalizer", updated.Finalizers)
	}

	// Second reconcile should proceed with sync
	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	if omni.syncCalls != 1 {
		t.Fatalf("syncCalls = %d, want 1", omni.syncCalls)
	}
}

func TestChildControllerMarksMissingCluster(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	machine := &omniv1alpha1.OmniMachine{
		ObjectMeta: metav1.ObjectMeta{Name: testMachineName, Namespace: testNamespace},
		Spec: omniv1alpha1.OmniMachineSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: "missing"},
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniMachine{}).
		WithObjects(machine).
		Build()

	reconciler := &OmniMachineReconciler{Client: k8sClient}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testMachineName}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &omniv1alpha1.OmniMachine{}
	if err := k8sClient.Get(ctx, request.NamespacedName, updated); err != nil {
		t.Fatalf("get machine: %v", err)
	}
	if got := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionAccepted); got == nil || got.Status != metav1.ConditionFalse {
		t.Fatalf("Accepted condition = %#v, want False", got)
	}
}

func TestChildControllersMarkClusterAccepted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		object k8sObject
		setup  func(scheme *runtime.Scheme, object k8sObject) (client.Client, reconcilerFunc)
		assert func(t *testing.T, ctx context.Context, k8sClient clientReader)
	}{
		{
			name:   "control plane",
			object: testControlPlane(),
			setup: func(scheme *runtime.Scheme, object k8sObject) (client.Client, reconcilerFunc) {
				k8sClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithStatusSubresource(&omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniControlPlane{}).
					WithObjects(testCluster(), object).
					Build()
				reconciler := &OmniControlPlaneReconciler{Client: k8sClient}
				return k8sClient, reconciler.Reconcile
			},
			assert: func(t *testing.T, ctx context.Context, k8sClient clientReader) {
				t.Helper()
				updated := &omniv1alpha1.OmniControlPlane{}
				assertAccepted(t, ctx, k8sClient, types.NamespacedName{Namespace: testNamespace, Name: testControlPlaneName}, updated)
			},
		},
		{
			name:   "workers",
			object: testWorkers(),
			setup: func(scheme *runtime.Scheme, object k8sObject) (client.Client, reconcilerFunc) {
				k8sClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithStatusSubresource(&omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniWorkers{}).
					WithObjects(testCluster(), object).
					Build()
				reconciler := &OmniWorkersReconciler{Client: k8sClient}
				return k8sClient, reconciler.Reconcile
			},
			assert: func(t *testing.T, ctx context.Context, k8sClient clientReader) {
				t.Helper()
				updated := &omniv1alpha1.OmniWorkers{}
				assertAccepted(t, ctx, k8sClient, types.NamespacedName{Namespace: testNamespace, Name: testWorkersName}, updated)
			},
		},
		{
			name: "machine",
			object: &omniv1alpha1.OmniMachine{
				ObjectMeta: metav1.ObjectMeta{Name: testMachineName, Namespace: testNamespace},
				Spec: omniv1alpha1.OmniMachineSpec{
					ClusterRef: omniv1alpha1.OmniClusterRef{Name: testClusterName},
				},
			},
			setup: func(scheme *runtime.Scheme, object k8sObject) (client.Client, reconcilerFunc) {
				k8sClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithStatusSubresource(&omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniMachine{}).
					WithObjects(testCluster(), object).
					Build()
				reconciler := &OmniMachineReconciler{Client: k8sClient}
				return k8sClient, reconciler.Reconcile
			},
			assert: func(t *testing.T, ctx context.Context, k8sClient clientReader) {
				t.Helper()
				updated := &omniv1alpha1.OmniMachine{}
				assertAccepted(t, ctx, k8sClient, types.NamespacedName{Namespace: testNamespace, Name: testMachineName}, updated)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			scheme := testScheme(t)
			k8sClient, reconcile := tt.setup(scheme, tt.object)

			key := types.NamespacedName{Namespace: tt.object.GetNamespace(), Name: tt.object.GetName()}
			if _, err := reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}

			tt.assert(t, ctx, k8sClient)
		})
	}
}

func TestOmniConnectionReconcilesReachability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pingErr      error
		wantStatus   metav1.ConditionStatus
		wantReason   string
		wantStalled  metav1.ConditionStatus
		wantRequeue  bool
		wantError    bool
		wantContains string
	}{
		{
			name:         "success",
			wantStatus:   metav1.ConditionTrue,
			wantReason:   omniv1alpha1.ReasonConnectionReady,
			wantStalled:  metav1.ConditionFalse,
			wantRequeue:  true,
			wantContains: "connected to https://omni.example.test",
		},
		{
			name:         "failure",
			pingErr:      fmt.Errorf("unauthorized"),
			wantStatus:   metav1.ConditionFalse,
			wantReason:   omniv1alpha1.ReasonConnectionFailed,
			wantStalled:  metav1.ConditionTrue,
			wantRequeue:  true,
			wantError:    true,
			wantContains: "failed to connect to Omni: unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			scheme := testScheme(t)
			connection := testConnection()
			connection.Finalizers = []string{omniv1alpha1.Finalizer}
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&omniv1alpha1.OmniConnection{}).
				WithObjects(connection).
				Build()

			omni := &fakeOmni{pingErr: tt.pingErr}
			reconciler := &OmniConnectionReconciler{Client: k8sClient, Omni: omni}
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: connection.Name}})
			if tt.wantError && err == nil {
				t.Fatal("Reconcile() error = nil, want error")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("Reconcile() error = %v, want nil", err)
			}
			if tt.wantRequeue && result.RequeueAfter == 0 {
				t.Fatal("RequeueAfter = 0, want periodic connection check")
			}
			if omni.pingCalls != 1 {
				t.Fatalf("pingCalls = %d, want 1", omni.pingCalls)
			}

			updated := &omniv1alpha1.OmniConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: connection.Name}, updated); err != nil {
				t.Fatalf("get connection: %v", err)
			}
			if updated.Status.LastCheckTime == nil {
				t.Fatal("LastCheckTime is nil")
			}
			if updated.Status.Endpoint != connection.Spec.Endpoint {
				t.Fatalf("Endpoint = %q, want %q", updated.Status.Endpoint, connection.Spec.Endpoint)
			}
			ready := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionReady)
			if ready == nil || ready.Status != tt.wantStatus || ready.Reason != tt.wantReason || !strings.Contains(ready.Message, tt.wantContains) {
				t.Fatalf("Ready condition = %#v, want status %s reason %s containing %q", ready, tt.wantStatus, tt.wantReason, tt.wantContains)
			}
			reachable := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionReachable)
			if reachable == nil || reachable.Status != tt.wantStatus || reachable.Reason != tt.wantReason {
				t.Fatalf("Reachable condition = %#v, want status %s reason %s", reachable, tt.wantStatus, tt.wantReason)
			}
			stalled := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionStalled)
			if stalled == nil || stalled.Status != tt.wantStalled || stalled.Reason != tt.wantReason {
				t.Fatalf("Stalled condition = %#v, want status %s reason %s", stalled, tt.wantStalled, tt.wantReason)
			}
		})
	}
}

func TestOmniConnectionAddsFinalizer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	connection := testConnection()
	omni := &fakeOmni{}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniConnection{}).
		WithObjects(connection).
		Build()
	reconciler := &OmniConnectionReconciler{Client: k8sClient, Omni: omni}

	result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: connection.Name}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result == (ctrl.Result{}) {
		t.Fatal("Result is empty, want requeue after adding finalizer")
	}
	if omni.pingCalls != 0 {
		t.Fatalf("pingCalls = %d, want 0 before finalizer is persisted", omni.pingCalls)
	}

	updated := &omniv1alpha1.OmniConnection{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: connection.Name}, updated); err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if len(updated.Finalizers) != 1 || updated.Finalizers[0] != omniv1alpha1.Finalizer {
		t.Fatalf("finalizers = %#v, want new finalizer", updated.Finalizers)
	}
}

func TestOmniConnectionDeleteWaitsForReferencingClusters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	deletionTime := metav1.Now()
	connection := testConnection()
	connection.Finalizers = []string{omniv1alpha1.Finalizer}
	connection.DeletionTimestamp = &deletionTime
	cluster := testCluster()
	otherCluster := testCluster()
	otherCluster.Name = testOtherClusterName
	otherCluster.Spec.ConnectionRef.Name = testOtherConnection
	omni := &fakeOmni{}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniConnection{}).
		WithObjects(connection, cluster, otherCluster).
		Build()
	reconciler := &OmniConnectionReconciler{Client: k8sClient, Omni: omni}

	result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: connection.Name}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter != time.Minute {
		t.Fatalf("RequeueAfter = %s, want 1m", result.RequeueAfter)
	}
	if omni.pingCalls != 0 {
		t.Fatalf("pingCalls = %d, want 0 during deletion", omni.pingCalls)
	}

	updated := &omniv1alpha1.OmniConnection{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: connection.Name}, updated); err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if len(updated.Finalizers) != 1 || updated.Finalizers[0] != omniv1alpha1.Finalizer {
		t.Fatalf("finalizers = %#v, want finalizer retained", updated.Finalizers)
	}
	ready := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != omniv1alpha1.ReasonDeleting || !strings.Contains(ready.Message, testClusterName) || strings.Contains(ready.Message, testOtherClusterName) {
		t.Fatalf("Ready condition = %#v, want blocked by %q only", ready, testClusterName)
	}
	stalled := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionStalled)
	if stalled == nil || stalled.Status != metav1.ConditionTrue || stalled.Reason != omniv1alpha1.ReasonDeleting {
		t.Fatalf("Stalled condition = %#v, want True/%s", stalled, omniv1alpha1.ReasonDeleting)
	}
}

func TestOmniConnectionDeleteRemovesFinalizersWhenUnreferenced(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	deletionTime := metav1.Now()
	connection := testConnection()
	connection.Finalizers = []string{omniv1alpha1.Finalizer, omniv1alpha1.LegacyFinalizer}
	connection.DeletionTimestamp = &deletionTime
	omni := &fakeOmni{}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniConnection{}).
		WithObjects(connection).
		Build()
	reconciler := &OmniConnectionReconciler{Client: k8sClient, Omni: omni}

	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: connection.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if omni.pingCalls != 0 {
		t.Fatalf("pingCalls = %d, want 0 during deletion", omni.pingCalls)
	}

	updated := &omniv1alpha1.OmniConnection{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: connection.Name}, updated)
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if len(updated.Finalizers) != 0 {
		t.Fatalf("finalizers = %#v, want empty", updated.Finalizers)
	}
}

func TestOmniHelmReleaseInstallsDirectRelease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	item := testHelmRelease()
	secret := testHelmReleaseKubeconfigSecret(item, testKubeconfigBytes("helm-token"))
	helm := &fakeHelmReleaseClient{}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniHelmRelease{}).
		WithObjects(testCluster(), item, secret).
		Build()
	reconciler := &OmniHelmReleaseReconciler{Client: k8sClient, Helm: helm}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if helm.reconcileCalls != 0 {
		t.Fatalf("reconcileCalls after finalizer = %d, want 0", helm.reconcileCalls)
	}
	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if helm.reconcileCalls != 1 {
		t.Fatalf("reconcileCalls = %d, want 1", helm.reconcileCalls)
	}
	if got := string(helm.reconcileConfigs[0]); !strings.Contains(got, "helm-token") {
		t.Fatalf("kubeconfig passed to Helm missing token: %s", got)
	}

	updated := &omniv1alpha1.OmniHelmRelease{}
	if err := k8sClient.Get(ctx, request.NamespacedName, updated); err != nil {
		t.Fatalf("get helm release: %v", err)
	}
	if updated.Status.LastAction != helmrelease.ActionInstall {
		t.Fatalf("LastAction = %q, want %q", updated.Status.LastAction, helmrelease.ActionInstall)
	}
	if updated.Status.ReleaseRevision != 1 {
		t.Fatalf("ReleaseRevision = %d, want 1", updated.Status.ReleaseRevision)
	}
	if updated.Status.LastSuccessTime == nil {
		t.Fatal("LastSuccessTime is nil")
	}
	if updated.Status.LastError != "" {
		t.Fatalf("LastError = %q, want empty", updated.Status.LastError)
	}
	if got := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionAccepted); got == nil || got.Status != metav1.ConditionTrue {
		t.Fatalf("Accepted condition = %#v, want True", got)
	}
	if got := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionReleased); got == nil || got.Status != metav1.ConditionTrue || got.Reason != omniv1alpha1.ReasonHelmInstalled {
		t.Fatalf("Released condition = %#v, want True/%s", got, omniv1alpha1.ReasonHelmInstalled)
	}
	if got := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionReady); got == nil || got.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition = %#v, want True", got)
	}
}

func TestOmniHelmReleaseReportsUpgradeStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	item := testHelmRelease()
	item.Finalizers = []string{omniv1alpha1.Finalizer}
	secret := testHelmReleaseKubeconfigSecret(item, testKubeconfigBytes("helm-token"))
	helm := &fakeHelmReleaseClient{
		reconcileResult: &helmrelease.Result{
			Action:       helmrelease.ActionUpgrade,
			ReleaseName:  testHelmReleaseName,
			Namespace:    testHelmNamespace,
			Chart:        testHelmReleaseName,
			ChartVersion: "3.14.0",
			Revision:     4,
			Status:       helmrelease.StatusDeployed,
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniHelmRelease{}).
		WithObjects(testCluster(), item, secret).
		Build()
	reconciler := &OmniHelmReleaseReconciler{Client: k8sClient, Helm: helm}

	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if helm.reconcileCalls != 1 {
		t.Fatalf("reconcileCalls = %d, want 1", helm.reconcileCalls)
	}

	updated := &omniv1alpha1.OmniHelmRelease{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Name}, updated); err != nil {
		t.Fatalf("get helm release: %v", err)
	}
	if updated.Status.LastAction != helmrelease.ActionUpgrade {
		t.Fatalf("LastAction = %q, want %q", updated.Status.LastAction, helmrelease.ActionUpgrade)
	}
	if updated.Status.ReleaseRevision != 4 {
		t.Fatalf("ReleaseRevision = %d, want 4", updated.Status.ReleaseRevision)
	}
	if got := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionReleased); got == nil || got.Status != metav1.ConditionTrue || got.Reason != omniv1alpha1.ReasonHelmUpgraded {
		t.Fatalf("Released condition = %#v, want True/%s", got, omniv1alpha1.ReasonHelmUpgraded)
	}
}

func TestOmniHelmReleaseReportsHelmFailure(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		action string
	}{
		{name: "install", action: helmrelease.ActionInstall},
		{name: "upgrade", action: helmrelease.ActionUpgrade},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			scheme := testScheme(t)
			item := testHelmRelease()
			item.Finalizers = []string{omniv1alpha1.Finalizer}
			secret := testHelmReleaseKubeconfigSecret(item, testKubeconfigBytes("helm-token"))
			helm := &fakeHelmReleaseClient{
				reconcileResult: &helmrelease.Result{
					Action:       tt.action,
					ReleaseName:  testHelmReleaseName,
					Namespace:    testHelmNamespace,
					Chart:        testHelmReleaseName,
					ChartVersion: testHelmChartVersion,
					Revision:     3,
					Status:       "failed",
				},
				reconcileErr: errors.New("helm action failed"),
			}
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&omniv1alpha1.OmniHelmRelease{}).
				WithObjects(testCluster(), item, secret).
				Build()
			reconciler := &OmniHelmReleaseReconciler{Client: k8sClient, Helm: helm}

			if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}}); err == nil || !strings.Contains(err.Error(), "helm action failed") {
				t.Fatalf("Reconcile() error = %v, want helm action failed", err)
			}

			updated := &omniv1alpha1.OmniHelmRelease{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Name}, updated); err != nil {
				t.Fatalf("get helm release: %v", err)
			}
			if updated.Status.LastAction != tt.action {
				t.Fatalf("LastAction = %q, want %q", updated.Status.LastAction, tt.action)
			}
			if updated.Status.LastError != "helm action failed" {
				t.Fatalf("LastError = %q, want helm action failed", updated.Status.LastError)
			}
			if got := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionReady); got == nil || got.Status != metav1.ConditionFalse || got.Reason != omniv1alpha1.ReasonReconcileFailed {
				t.Fatalf("Ready condition = %#v, want False/%s", got, omniv1alpha1.ReasonReconcileFailed)
			}
		})
	}
}

func TestOmniHelmReleaseWaitsForClusterAndCredentials(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		objects    []client.Object
		wantReason string
	}{
		{
			name:       "missing cluster",
			objects:    nil,
			wantReason: omniv1alpha1.ReasonMissingCluster,
		},
		{
			name:       "missing kubeconfig secret",
			objects:    []client.Object{testCluster()},
			wantReason: omniv1alpha1.ReasonMissingSecret,
		},
		{
			name: "missing kubeconfig key",
			objects: []client.Object{
				testCluster(),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "edge-helm-kubeconfig", Namespace: testNamespace},
					Data:       map[string][]byte{"other": testKubeconfigBytes("helm-token")},
				},
			},
			wantReason: omniv1alpha1.ReasonMissingSecret,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			scheme := testScheme(t)
			item := testHelmRelease()
			item.Finalizers = []string{omniv1alpha1.Finalizer}
			objects := append([]client.Object{item}, tt.objects...)
			helm := &fakeHelmReleaseClient{}
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&omniv1alpha1.OmniHelmRelease{}).
				WithObjects(objects...).
				Build()
			reconciler := &OmniHelmReleaseReconciler{Client: k8sClient, Helm: helm}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}})
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if result.RequeueAfter != helmReleaseRetryInterval {
				t.Fatalf("RequeueAfter = %s, want %s", result.RequeueAfter, helmReleaseRetryInterval)
			}
			if helm.reconcileCalls != 0 {
				t.Fatalf("reconcileCalls = %d, want 0", helm.reconcileCalls)
			}

			updated := &omniv1alpha1.OmniHelmRelease{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Name}, updated); err != nil {
				t.Fatalf("get helm release: %v", err)
			}
			if got := meta.FindStatusCondition(updated.Status.Conditions, omniv1alpha1.ConditionReady); got == nil || got.Status != metav1.ConditionFalse || got.Reason != tt.wantReason {
				t.Fatalf("Ready condition = %#v, want False/%s", got, tt.wantReason)
			}
		})
	}
}

func TestOmniHelmReleaseDeleteUninstallsRelease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	deletionTime := metav1.Now()
	item := testHelmRelease()
	item.Finalizers = []string{omniv1alpha1.Finalizer}
	item.DeletionTimestamp = &deletionTime
	secret := testHelmReleaseKubeconfigSecret(item, testKubeconfigBytes("helm-token"))
	helm := &fakeHelmReleaseClient{}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniHelmRelease{}).
		WithObjects(testCluster(), item, secret).
		Build()
	reconciler := &OmniHelmReleaseReconciler{Client: k8sClient, Helm: helm}

	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if helm.uninstallCalls != 1 {
		t.Fatalf("uninstallCalls = %d, want 1", helm.uninstallCalls)
	}

	updated := &omniv1alpha1.OmniHelmRelease{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Name}, updated)
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		t.Fatalf("get helm release: %v", err)
	}
	if len(updated.Finalizers) != 0 {
		t.Fatalf("finalizers = %#v, want empty", updated.Finalizers)
	}
}

func TestOmniHelmReleaseDeleteCanOrphanRelease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	deletionTime := metav1.Now()
	item := testHelmRelease()
	item.Finalizers = []string{omniv1alpha1.Finalizer}
	item.DeletionTimestamp = &deletionTime
	item.Spec.DeletionPolicy = omniv1alpha1.HelmReleaseDeletionPolicyOrphan
	helm := &fakeHelmReleaseClient{}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniHelmRelease{}).
		WithObjects(testCluster(), item).
		Build()
	reconciler := &OmniHelmReleaseReconciler{Client: k8sClient, Helm: helm}

	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if helm.uninstallCalls != 0 {
		t.Fatalf("uninstallCalls = %d, want 0", helm.uninstallCalls)
	}

	updated := &omniv1alpha1.OmniHelmRelease{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Name}, updated)
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		t.Fatalf("get helm release: %v", err)
	}
	if len(updated.Finalizers) != 0 {
		t.Fatalf("finalizers = %#v, want empty", updated.Finalizers)
	}
}

func TestOmniKubeconfigExportWritesServiceAccountSecret(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	omni := &fakeOmni{kubeconfigData: testKubeconfigBytes("first-token")}
	item := testKubeconfigExport()
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniKubeconfigExport{}, &omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniConnection{}).
		WithObjects(testConnection(), testCluster(), item).
		Build()
	reconciler := &OmniKubeconfigExportReconciler{
		Client: k8sClient,
		Omni:   omni,
		Clock:  func() time.Time { return now },
	}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}}

	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	result, err := reconciler.Reconcile(ctx, request)
	if err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	if result.RequeueAfter != 20*time.Hour {
		t.Fatalf("RequeueAfter = %s, want 20h", result.RequeueAfter)
	}
	if omni.kubeconfigCalls != 1 {
		t.Fatalf("kubeconfigCalls = %d, want 1", omni.kubeconfigCalls)
	}
	if omni.kubeconfigCluster != testClusterName {
		t.Fatalf("kubeconfigCluster = %q, want %q", omni.kubeconfigCluster, testClusterName)
	}
	if len(omni.kubeconfigOptions) != 1 || omni.kubeconfigOptions[0].TTL != 24*time.Hour || omni.kubeconfigOptions[0].User != "edge-automation" || strings.Join(omni.kubeconfigOptions[0].Groups, ",") != "cluster-automation" {
		t.Fatalf("kubeconfigOptions = %#v, want scoped service account options", omni.kubeconfigOptions)
	}

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Namespace: testNamespace, Name: item.Spec.TargetSecretRef.Name}
	if err := k8sClient.Get(ctx, secretKey, secret); err != nil {
		t.Fatalf("get exported Secret: %v", err)
	}
	if string(secret.Data[kubeconfigexport.DefaultSecretKey]) != string(testKubeconfigBytes("first-token")) {
		t.Fatalf("Secret kubeconfig data = %q, want first-token kubeconfig", string(secret.Data[kubeconfigexport.DefaultSecretKey]))
	}
	if secret.Labels[kubeconfigexport.OwnerUIDLabel] != string(item.UID) {
		t.Fatalf("owner UID label = %q, want %q", secret.Labels[kubeconfigexport.OwnerUIDLabel], item.UID)
	}
	if secret.Annotations[kubeconfigexport.HashAnnotation] != kubeconfigexport.Hash(testKubeconfigBytes("first-token")) {
		t.Fatalf("hash annotation = %q, want kubeconfig hash", secret.Annotations[kubeconfigexport.HashAnnotation])
	}

	latest := &omniv1alpha1.OmniKubeconfigExport{}
	if err := k8sClient.Get(ctx, request.NamespacedName, latest); err != nil {
		t.Fatalf("get export: %v", err)
	}
	if got := meta.FindStatusCondition(latest.Status.Conditions, omniv1alpha1.ConditionReady); got == nil || got.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition = %#v, want True", got)
	}
	if latest.Status.ExpirationTime == nil || !latest.Status.ExpirationTime.Time.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("ExpirationTime = %#v, want %s", latest.Status.ExpirationTime, now.Add(24*time.Hour))
	}
	if latest.Status.NextRotationTime == nil || !latest.Status.NextRotationTime.Time.Equal(now.Add(20*time.Hour)) {
		t.Fatalf("NextRotationTime = %#v, want %s", latest.Status.NextRotationTime, now.Add(20*time.Hour))
	}
}

func TestOmniKubeconfigExportReusesCurrentSecretUntilRenewBefore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	item := testKubeconfigExport()
	item.Finalizers = []string{omniv1alpha1.Finalizer}
	data := testKubeconfigBytes("current-token")
	expirationTime := metav1.NewTime(now.Add(24 * time.Hour))
	lastRotationTime := metav1.NewTime(now.Add(-time.Hour))
	secret := currentKubeconfigExportSecret(t, item, data, expirationTime, lastRotationTime)
	omni := &fakeOmni{kubeconfigData: testKubeconfigBytes("rotated-token")}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniKubeconfigExport{}, &omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniConnection{}).
		WithObjects(testConnection(), testCluster(), item, secret).
		Build()
	reconciler := &OmniKubeconfigExportReconciler{
		Client: k8sClient,
		Omni:   omni,
		Clock:  func() time.Time { return now },
	}

	result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if omni.kubeconfigCalls != 0 {
		t.Fatalf("kubeconfigCalls = %d, want 0", omni.kubeconfigCalls)
	}
	if result.RequeueAfter != 20*time.Hour {
		t.Fatalf("RequeueAfter = %s, want 20h", result.RequeueAfter)
	}

	latest := &omniv1alpha1.OmniKubeconfigExport{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Name}, latest); err != nil {
		t.Fatalf("get export: %v", err)
	}
	if latest.Status.KubeconfigHash != kubeconfigexport.Hash(data) {
		t.Fatalf("KubeconfigHash = %q, want current hash", latest.Status.KubeconfigHash)
	}
}

func TestOmniKubeconfigExportRotatesWhenRenewBeforeElapsed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	item := testKubeconfigExport()
	item.Finalizers = []string{omniv1alpha1.Finalizer}
	oldData := testKubeconfigBytes("old-token")
	expirationTime := metav1.NewTime(now.Add(3 * time.Hour))
	lastRotationTime := metav1.NewTime(now.Add(-21 * time.Hour))
	secret := currentKubeconfigExportSecret(t, item, oldData, expirationTime, lastRotationTime)
	omni := &fakeOmni{kubeconfigData: testKubeconfigBytes("rotated-token")}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniKubeconfigExport{}, &omniv1alpha1.OmniCluster{}, &omniv1alpha1.OmniConnection{}).
		WithObjects(testConnection(), testCluster(), item, secret).
		Build()
	reconciler := &OmniKubeconfigExportReconciler{
		Client: k8sClient,
		Omni:   omni,
		Clock:  func() time.Time { return now },
	}

	result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if omni.kubeconfigCalls != 1 {
		t.Fatalf("kubeconfigCalls = %d, want 1", omni.kubeconfigCalls)
	}
	if result.RequeueAfter != 20*time.Hour {
		t.Fatalf("RequeueAfter = %s, want 20h", result.RequeueAfter)
	}

	updated := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Spec.TargetSecretRef.Name}, updated); err != nil {
		t.Fatalf("get exported Secret: %v", err)
	}
	if string(updated.Data[kubeconfigexport.DefaultSecretKey]) != string(testKubeconfigBytes("rotated-token")) {
		t.Fatalf("Secret kubeconfig data was not rotated")
	}
}

func TestOmniKubeconfigExportDeletionPolicy(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		policy     omniv1alpha1.KubeconfigExportDeletionPolicy
		wantSecret bool
	}{
		{name: "delete", policy: omniv1alpha1.KubeconfigExportDeletionPolicyDelete, wantSecret: false},
		{name: "orphan", policy: omniv1alpha1.KubeconfigExportDeletionPolicyOrphan, wantSecret: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			scheme := testScheme(t)
			item := testKubeconfigExport()
			item.Spec.DeletionPolicy = tt.policy
			item.Finalizers = []string{omniv1alpha1.Finalizer}
			deletionTime := metav1.Now()
			item.DeletionTimestamp = &deletionTime
			secret := currentKubeconfigExportSecret(t, item, testKubeconfigBytes("delete-token"), metav1.NewTime(time.Now().Add(time.Hour)), metav1.Now())
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&omniv1alpha1.OmniKubeconfigExport{}).
				WithObjects(item, secret).
				Build()
			reconciler := &OmniKubeconfigExportReconciler{Client: k8sClient}

			if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}}); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}

			gotSecret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Spec.TargetSecretRef.Name}, gotSecret)
			if tt.wantSecret && err != nil {
				t.Fatalf("get orphaned Secret: %v", err)
			}
			if !tt.wantSecret && !apierrors.IsNotFound(err) {
				t.Fatalf("deleted Secret get error = %v, want NotFound", err)
			}
		})
	}
}

func TestOmniKubeconfigExportReportsMissingDependencies(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		objects    []client.Object
		wantReason string
	}{
		{
			name:       "missing cluster",
			objects:    []client.Object{testConnection()},
			wantReason: omniv1alpha1.ReasonMissingCluster,
		},
		{
			name:       "missing connection",
			objects:    []client.Object{testCluster()},
			wantReason: omniv1alpha1.ReasonMissingConnection,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			scheme := testScheme(t)
			item := testKubeconfigExport()
			item.Finalizers = []string{omniv1alpha1.Finalizer}
			objects := append([]client.Object{item}, tt.objects...)
			omni := &fakeOmni{}
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&omniv1alpha1.OmniKubeconfigExport{}).
				WithObjects(objects...).
				Build()
			reconciler := &OmniKubeconfigExportReconciler{
				Client: k8sClient,
				Omni:   omni,
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}})
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if result.RequeueAfter != kubeconfigExportRetryInterval {
				t.Fatalf("RequeueAfter = %s, want %s", result.RequeueAfter, kubeconfigExportRetryInterval)
			}
			if omni.kubeconfigCalls != 0 {
				t.Fatalf("kubeconfigCalls = %d, want 0", omni.kubeconfigCalls)
			}

			latest := &omniv1alpha1.OmniKubeconfigExport{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Name}, latest); err != nil {
				t.Fatalf("get export: %v", err)
			}
			ready := meta.FindStatusCondition(latest.Status.Conditions, omniv1alpha1.ConditionReady)
			if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != tt.wantReason {
				t.Fatalf("Ready condition = %#v, want False/%s", ready, tt.wantReason)
			}
			if latest.Status.TargetSecretRef != item.Spec.TargetSecretRef.Name {
				t.Fatalf("TargetSecretRef status = %q, want %q", latest.Status.TargetSecretRef, item.Spec.TargetSecretRef.Name)
			}

			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Spec.TargetSecretRef.Name}, secret)
			if !apierrors.IsNotFound(err) {
				t.Fatalf("target Secret get error = %v, want NotFound", err)
			}
		})
	}
}

func TestOmniKubeconfigExportReportsExportFailures(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		omni *fakeOmni
	}{
		{
			name: "omni error",
			omni: &fakeOmni{kubeconfigErr: errors.New("omni unavailable")},
		},
		{
			name: "invalid kubeconfig",
			omni: &fakeOmni{kubeconfigData: []byte("apiVersion: v1\nkind: Config\nusers: [")},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			scheme := testScheme(t)
			item := testKubeconfigExport()
			item.Finalizers = []string{omniv1alpha1.Finalizer}
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&omniv1alpha1.OmniKubeconfigExport{}).
				WithObjects(testConnection(), testCluster(), item).
				Build()
			reconciler := &OmniKubeconfigExportReconciler{
				Client: k8sClient,
				Omni:   tt.omni,
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}})
			if err == nil {
				t.Fatal("Reconcile() error = nil, want export error")
			}
			if result.RequeueAfter != kubeconfigExportRetryInterval {
				t.Fatalf("RequeueAfter = %s, want %s", result.RequeueAfter, kubeconfigExportRetryInterval)
			}
			if tt.omni.kubeconfigCalls != 1 {
				t.Fatalf("kubeconfigCalls = %d, want 1", tt.omni.kubeconfigCalls)
			}

			latest := &omniv1alpha1.OmniKubeconfigExport{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Name}, latest); err != nil {
				t.Fatalf("get export: %v", err)
			}
			ready := meta.FindStatusCondition(latest.Status.Conditions, omniv1alpha1.ConditionReady)
			if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != omniv1alpha1.ReasonExportFailed {
				t.Fatalf("Ready condition = %#v, want False/%s", ready, omniv1alpha1.ReasonExportFailed)
			}

			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Spec.TargetSecretRef.Name}, secret)
			if !apierrors.IsNotFound(err) {
				t.Fatalf("target Secret get error = %v, want NotFound", err)
			}
		})
	}
}

func TestOmniKubeconfigExportCleansPreviousOwnedTargetSecret(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	item := testKubeconfigExport()
	item.Finalizers = []string{omniv1alpha1.Finalizer}
	item.Status.TargetSecretRef = testOldKubeconfigKey
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testOldKubeconfigKey,
			Namespace: testNamespace,
			Labels: map[string]string{
				kubeconfigexport.OwnerUIDLabel: string(item.UID),
			},
			Annotations: map[string]string{
				kubeconfigexport.OwnerAnnotation: item.Name,
			},
		},
		Data: map[string][]byte{
			kubeconfigexport.DefaultSecretKey: testKubeconfigBytes("old-token"),
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniKubeconfigExport{}).
		WithObjects(testConnection(), testCluster(), item, oldSecret).
		Build()
	reconciler := &OmniKubeconfigExportReconciler{
		Client: k8sClient,
		Omni:   &fakeOmni{kubeconfigData: testKubeconfigBytes("new-token")},
		Clock:  func() time.Time { return now },
	}

	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testOldKubeconfigKey}, &corev1.Secret{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("old Secret get error = %v, want NotFound", err)
	}
	newSecret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Spec.TargetSecretRef.Name}, newSecret); err != nil {
		t.Fatalf("get new Secret: %v", err)
	}
	if string(newSecret.Data[kubeconfigexport.DefaultSecretKey]) != string(testKubeconfigBytes("new-token")) {
		t.Fatal("new Secret does not contain rotated kubeconfig")
	}
}

func TestOmniKubeconfigExportRemovesPreviousKeyInOwnedSecret(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	item := testKubeconfigExport()
	item.Finalizers = []string{omniv1alpha1.Finalizer}
	item.Status.TargetSecretRef = item.Spec.TargetSecretRef.Name
	item.Status.TargetSecretKey = testOldKubeconfigKey
	item.Spec.TargetSecretRef.Key = "new-kubeconfig"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      item.Spec.TargetSecretRef.Name,
			Namespace: testNamespace,
			Labels: map[string]string{
				kubeconfigexport.OwnerUIDLabel: string(item.UID),
			},
			Annotations: map[string]string{
				kubeconfigexport.OwnerAnnotation: item.Name,
			},
		},
		Data: map[string][]byte{
			testOldKubeconfigKey: testKubeconfigBytes("old-token"),
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniKubeconfigExport{}).
		WithObjects(testConnection(), testCluster(), item, secret).
		Build()
	reconciler := &OmniKubeconfigExportReconciler{
		Client: k8sClient,
		Omni:   &fakeOmni{kubeconfigData: testKubeconfigBytes("new-token")},
		Clock:  func() time.Time { return now },
	}

	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: item.Name}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: item.Spec.TargetSecretRef.Name}, updated); err != nil {
		t.Fatalf("get Secret: %v", err)
	}
	if _, ok := updated.Data[testOldKubeconfigKey]; ok {
		t.Fatal("old key is still present")
	}
	if string(updated.Data["new-kubeconfig"]) != string(testKubeconfigBytes("new-token")) {
		t.Fatal("new key does not contain exported kubeconfig")
	}
}

func TestSpecOrDeletionChangedPredicateIgnoresStatusOnlyUpdates(t *testing.T) {
	t.Parallel()

	predicate := specOrDeletionChangedPredicate()
	oldCluster := testCluster()
	oldCluster.Generation = 7
	newCluster := oldCluster.DeepCopy()
	newCluster.Status.ClusterName = testClusterName

	if predicate.Update(event.UpdateEvent{ObjectOld: oldCluster, ObjectNew: newCluster}) {
		t.Fatal("status-only update should be ignored")
	}

	specChanged := oldCluster.DeepCopy()
	specChanged.Generation = 8
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldCluster, ObjectNew: specChanged}) {
		t.Fatal("generation update should be accepted")
	}

	deleting := oldCluster.DeepCopy()
	deletionTime := metav1.Now()
	deleting.DeletionTimestamp = &deletionTime
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldCluster, ObjectNew: deleting}) {
		t.Fatal("deletion timestamp update should be accepted")
	}
}

func TestChildRequestsForCluster(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	otherControlPlane := testControlPlane()
	otherControlPlane.Name = "other-cp"
	otherControlPlane.Spec.ClusterRef.Name = testOtherClusterName
	workers := testWorkers()
	machine := &omniv1alpha1.OmniMachine{
		ObjectMeta: metav1.ObjectMeta{Name: testMachineName, Namespace: testNamespace},
		Spec: omniv1alpha1.OmniMachineSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: testClusterName},
		},
	}
	cluster := testCluster()
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testControlPlane(), otherControlPlane, workers, machine).
		Build()

	controlPlaneRequests := controlPlaneRequestsForCluster(ctx, k8sClient, cluster)
	if len(controlPlaneRequests) != 1 || controlPlaneRequests[0].Name != testControlPlaneName {
		t.Fatalf("controlPlaneRequests = %#v, want [edge-cp]", controlPlaneRequests)
	}

	workersRequests := workersRequestsForCluster(ctx, k8sClient, cluster)
	if len(workersRequests) != 1 || workersRequests[0].Name != testWorkersName {
		t.Fatalf("workersRequests = %#v, want [workers]", workersRequests)
	}

	machineRequests := machineRequestsForCluster(ctx, k8sClient, cluster)
	if len(machineRequests) != 1 || machineRequests[0].Name != testMachineName {
		t.Fatalf("machineRequests = %#v, want [node-1]", machineRequests)
	}

}

func TestClusterWatchRequestMapping(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			testCluster(),
			&omniv1alpha1.OmniCluster{
				ObjectMeta: metav1.ObjectMeta{Name: testOtherClusterName, Namespace: testNamespace},
				Spec: omniv1alpha1.OmniClusterSpec{
					ConnectionRef: omniv1alpha1.OmniConnectionRef{Name: testOtherConnection},
				},
			},
		).
		Build()
	reconciler := &OmniClusterReconciler{Client: k8sClient}

	for _, child := range []client.Object{testControlPlane(), testWorkers(), testMachine()} {
		requests := requestForChildCluster(ctx, child)
		if len(requests) != 1 || requests[0].Name != testClusterName {
			t.Fatalf("requestForChildCluster(%T) = %#v, want cluster %q", child, requests, testClusterName)
		}
	}
	if requests := requestForChildCluster(ctx, testConnection()); len(requests) != 0 {
		t.Fatalf("requestForChildCluster(connection) = %#v, want none", requests)
	}
	connectionRequests := requestForClusterConnection(ctx, testCluster())
	if len(connectionRequests) != 1 || connectionRequests[0].Name != testConnectionName {
		t.Fatalf("requestForClusterConnection() = %#v, want connection %q", connectionRequests, testConnectionName)
	}
	if requests := requestForClusterConnection(ctx, testConnection()); len(requests) != 0 {
		t.Fatalf("requestForClusterConnection(connection) = %#v, want none", requests)
	}
	emptyConnectionRef := testCluster()
	emptyConnectionRef.Spec.ConnectionRef.Name = ""
	if requests := requestForClusterConnection(ctx, emptyConnectionRef); len(requests) != 0 {
		t.Fatalf("requestForClusterConnection(empty) = %#v, want none", requests)
	}
	if requests := requestForClusterRef(testNamespace, ""); len(requests) != 0 {
		t.Fatalf("requestForClusterRef(empty) = %#v, want none", requests)
	}

	requests := reconciler.clustersForConnection(ctx, testConnection())
	if len(requests) != 1 || requests[0].Name != testClusterName {
		t.Fatalf("clustersForConnection() = %#v, want cluster %q", requests, testClusterName)
	}
}

func TestKubeconfigExportWatchRequestMapping(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	item := testKubeconfigExport()
	other := testKubeconfigExport()
	other.Name = "other-kubeconfig"
	other.Spec.ClusterRef.Name = testOtherClusterName
	otherCluster := &omniv1alpha1.OmniCluster{
		ObjectMeta: metav1.ObjectMeta{Name: testOtherClusterName, Namespace: testNamespace},
		Spec: omniv1alpha1.OmniClusterSpec{
			ConnectionRef: omniv1alpha1.OmniConnectionRef{Name: testOtherConnection},
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testCluster(), otherCluster, item, other).
		Build()
	reconciler := &OmniKubeconfigExportReconciler{Client: k8sClient}

	clusterRequests := kubeconfigExportRequestsForCluster(ctx, k8sClient, testCluster())
	if len(clusterRequests) != 1 || clusterRequests[0].Name != item.Name {
		t.Fatalf("kubeconfigExportRequestsForCluster() = %#v, want %q", clusterRequests, item.Name)
	}

	connectionRequests := reconciler.kubeconfigExportRequestsForConnection(ctx, testConnection())
	if len(connectionRequests) != 1 || connectionRequests[0].Name != item.Name {
		t.Fatalf("kubeconfigExportRequestsForConnection() = %#v, want %q", connectionRequests, item.Name)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      item.Spec.TargetSecretRef.Name,
			Namespace: item.Namespace,
			Annotations: map[string]string{
				kubeconfigexport.OwnerAnnotation: item.Name,
			},
		},
	}
	secretRequests := kubeconfigExportRequestsForSecret(ctx, secret)
	if len(secretRequests) != 1 || secretRequests[0].Name != item.Name {
		t.Fatalf("kubeconfigExportRequestsForSecret() = %#v, want %q", secretRequests, item.Name)
	}
	unannotated := secret.DeepCopy()
	unannotated.Annotations = nil
	if requests := kubeconfigExportRequestsForSecret(ctx, unannotated); len(requests) != 0 {
		t.Fatalf("kubeconfigExportRequestsForSecret(unannotated) = %#v, want none", requests)
	}
}

type k8sObject interface {
	client.Object
}

type clientReader interface {
	Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error
}

type reconcilerFunc func(context.Context, ctrl.Request) (ctrl.Result, error)

func assertAccepted(t *testing.T, ctx context.Context, k8sClient clientReader, key types.NamespacedName, object interface {
	client.Object
}) {
	t.Helper()

	if err := k8sClient.Get(ctx, key, object); err != nil {
		t.Fatalf("get %s: %v", key, err)
	}

	var conditions []metav1.Condition
	switch typed := object.(type) {
	case *omniv1alpha1.OmniControlPlane:
		conditions = typed.Status.Conditions
	case *omniv1alpha1.OmniWorkers:
		conditions = typed.Status.Conditions
	case *omniv1alpha1.OmniMachine:
		conditions = typed.Status.Conditions
	default:
		t.Fatalf("unsupported object type %T", object)
	}

	if got := meta.FindStatusCondition(conditions, omniv1alpha1.ConditionAccepted); got == nil || got.Status != metav1.ConditionTrue {
		t.Fatalf("Accepted condition = %#v, want True", got)
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := omniv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add omni scheme: %v", err)
	}

	return scheme
}

func testConnection() *omniv1alpha1.OmniConnection {
	return &omniv1alpha1.OmniConnection{
		ObjectMeta: metav1.ObjectMeta{Name: testConnectionName, Namespace: testNamespace},
		Spec: omniv1alpha1.OmniConnectionSpec{
			Endpoint: "https://omni.example.test",
			Auth: omniv1alpha1.OmniAuthSpec{
				ServiceAccountSecretRef: omniv1alpha1.SecretKeySelector{
					Name: "omni-service-account",
					Key:  "serviceAccountKey",
				},
			},
		},
	}
}

func testCluster() *omniv1alpha1.OmniCluster {
	return &omniv1alpha1.OmniCluster{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testNamespace},
		Spec: omniv1alpha1.OmniClusterSpec{
			ConnectionRef: omniv1alpha1.OmniConnectionRef{Name: testConnectionName},
			Kubernetes:    omniv1alpha1.KubernetesSpec{Version: "v1.35.0"},
			Talos:         omniv1alpha1.TalosSpec{Version: "v1.13.2"},
		},
	}
}

func testControlPlane() *omniv1alpha1.OmniControlPlane {
	return &omniv1alpha1.OmniControlPlane{
		ObjectMeta: metav1.ObjectMeta{Name: testControlPlaneName, Namespace: testNamespace},
		Spec: omniv1alpha1.OmniControlPlaneSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: testClusterName},
			MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
				MachineClass: &omniv1alpha1.MachineClass{
					Name: "control-plane",
					Size: intstr.FromInt32(1),
				},
			},
		},
	}
}

func testWorkers() *omniv1alpha1.OmniWorkers {
	return &omniv1alpha1.OmniWorkers{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkersName, Namespace: testNamespace},
		Spec: omniv1alpha1.OmniWorkersSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: testClusterName},
			MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
				MachineClass: &omniv1alpha1.MachineClass{
					Name: testWorkersName,
					Size: intstr.FromString("unlimited"),
				},
			},
		},
	}
}

func testHelmRelease() *omniv1alpha1.OmniHelmRelease {
	return &omniv1alpha1.OmniHelmRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-release", Namespace: testNamespace},
		Spec: omniv1alpha1.OmniHelmReleaseSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: testClusterName},
			KubeconfigSecretRef: omniv1alpha1.HelmReleaseKubeconfigSecretRef{
				Name: "edge-helm-kubeconfig",
			},
			ReleaseName:     testHelmReleaseName,
			Namespace:       testHelmNamespace,
			CreateNamespace: true,
			Wait:            true,
			WaitForJobs:     true,
			Timeout:         &metav1.Duration{Duration: 10 * time.Minute},
			Atomic:          true,
			Chart: omniv1alpha1.OmniHelmChartSpec{
				Repository: "https://kubernetes-sigs.github.io/metrics-server/",
				Chart:      testHelmReleaseName,
				Version:    testHelmChartVersion,
			},
		},
	}
}

func testMachine() *omniv1alpha1.OmniMachine {
	return &omniv1alpha1.OmniMachine{
		ObjectMeta: metav1.ObjectMeta{Name: testMachineName, Namespace: testNamespace},
		Spec: omniv1alpha1.OmniMachineSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: testClusterName},
			MachineID:  "33333333-3333-4333-8333-333333333333",
		},
	}
}

func testKubeconfigExport() *omniv1alpha1.OmniKubeconfigExport {
	return &omniv1alpha1.OmniKubeconfigExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "edge-automation-kubeconfig",
			Namespace: testNamespace,
			UID:       "11111111-1111-4111-8111-111111111111",
		},
		Spec: omniv1alpha1.OmniKubeconfigExportSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: testClusterName},
			TargetSecretRef: omniv1alpha1.KubeconfigTargetSecretRef{
				Name: "edge-automation-kubeconfig",
			},
			ServiceAccount: omniv1alpha1.KubeconfigServiceAccountSpec{
				User:   "edge-automation",
				Groups: []string{"cluster-automation"},
			},
			TTL:            metav1.Duration{Duration: 24 * time.Hour},
			RenewBefore:    &metav1.Duration{Duration: 4 * time.Hour},
			DeletionPolicy: omniv1alpha1.KubeconfigExportDeletionPolicyDelete,
		},
	}
}

func testKubeconfigBytes(token string) []byte {
	return fmt.Appendf(nil, `apiVersion: v1
kind: Config
clusters:
- name: edge
  cluster:
    server: https://edge.example.test
contexts:
- name: edge
  context:
    cluster: edge
    user: automation
current-context: edge
users:
- name: automation
  user:
    token: %s
`, token)
}

func currentKubeconfigExportSecret(t *testing.T, item *omniv1alpha1.OmniKubeconfigExport, data []byte, expirationTime, lastRotationTime metav1.Time) *corev1.Secret {
	t.Helper()

	specHash, err := kubeconfigexport.SpecHash(item, testClusterName)
	if err != nil {
		t.Fatalf("SpecHash() error = %v", err)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        item.Spec.TargetSecretRef.Name,
			Namespace:   item.Namespace,
			Labels:      kubeconfigexport.SecretLabels(item, testClusterName),
			Annotations: kubeconfigexport.SecretAnnotations(item, specHash, kubeconfigexport.Hash(data), expirationTime, lastRotationTime),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			kubeconfigexport.TargetSecretKey(item): data,
		},
	}
}

func testHelmReleaseKubeconfigSecret(item *omniv1alpha1.OmniHelmRelease, data []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: item.Spec.KubeconfigSecretRef.Name, Namespace: item.Namespace},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			helmrelease.KubeconfigSecretKey(item): data,
		},
	}
}
