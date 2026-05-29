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
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/omniapi"
)

const testClusterName = "edge"

type fakeOmni struct {
	pingErr        error
	syncErr        error
	statusErr      error
	syncedTemplate []byte
	syncOptions    []omniapi.SyncOptions
	syncCalls      int
	deleteCalls    []string
	deleteOptions  []omniapi.SyncOptions
}

func (f *fakeOmni) Ping(_ context.Context, connection *omniv1alpha1.OmniConnection) (string, error) {
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

	reconciler := &OmniClusterReconciler{Client: k8sClient, Scheme: scheme, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: testClusterName}}

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

	reconciler := &OmniClusterReconciler{Client: k8sClient, Scheme: scheme, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: testClusterName}}

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

	reconciler := &OmniClusterReconciler{Client: k8sClient, Scheme: scheme, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: testClusterName}}

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

	reconciler := &OmniClusterReconciler{Client: k8sClient, Scheme: scheme, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: testClusterName}}

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

	reconciler := &OmniClusterReconciler{Client: k8sClient, Scheme: scheme, Omni: omni}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: testClusterName}}

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

func TestChildControllerMarksMissingCluster(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scheme := testScheme(t)
	machine := &omniv1alpha1.OmniMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1", Namespace: "default"},
		Spec: omniv1alpha1.OmniMachineSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: "missing"},
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniv1alpha1.OmniMachine{}).
		WithObjects(machine).
		Build()

	reconciler := &OmniMachineReconciler{Client: k8sClient, Scheme: scheme}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "node-1"}}

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
	otherControlPlane.Spec.ClusterRef.Name = "other"
	workers := testWorkers()
	machine := &omniv1alpha1.OmniMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1", Namespace: "default"},
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
	if len(controlPlaneRequests) != 1 || controlPlaneRequests[0].Name != "edge-cp" {
		t.Fatalf("controlPlaneRequests = %#v, want [edge-cp]", controlPlaneRequests)
	}

	workersRequests := workersRequestsForCluster(ctx, k8sClient, cluster)
	if len(workersRequests) != 1 || workersRequests[0].Name != "workers" {
		t.Fatalf("workersRequests = %#v, want [workers]", workersRequests)
	}

	machineRequests := machineRequestsForCluster(ctx, k8sClient, cluster)
	if len(machineRequests) != 1 || machineRequests[0].Name != "node-1" {
		t.Fatalf("machineRequests = %#v, want [node-1]", machineRequests)
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
		ObjectMeta: metav1.ObjectMeta{Name: "omni", Namespace: "default"},
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
		ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: "default"},
		Spec: omniv1alpha1.OmniClusterSpec{
			ConnectionRef: omniv1alpha1.OmniConnectionRef{Name: "omni"},
			Kubernetes:    omniv1alpha1.KubernetesSpec{Version: "v1.35.0"},
			Talos:         omniv1alpha1.TalosSpec{Version: "v1.13.2"},
		},
	}
}

func testControlPlane() *omniv1alpha1.OmniControlPlane {
	return &omniv1alpha1.OmniControlPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "edge-cp", Namespace: "default"},
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
		ObjectMeta: metav1.ObjectMeta{Name: "workers", Namespace: "default"},
		Spec: omniv1alpha1.OmniWorkersSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: testClusterName},
			MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
				MachineClass: &omniv1alpha1.MachineClass{
					Name: "workers",
					Size: intstr.FromString("unlimited"),
				},
			},
		},
	}
}
