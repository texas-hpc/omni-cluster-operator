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

package v1alpha1

import (
	"context"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

func TestOmniClusterValidation(t *testing.T) {
	t.Parallel()

	validator := &OmniClusterCustomValidator{}
	cluster := validCluster()
	cluster.Spec.DeletePolicy.Orphan = true
	cluster.Spec.DeletePolicy.DestroyMachines = true

	_, err := validator.ValidateCreate(context.Background(), cluster)
	requireErrorContains(t, err, "orphan and destroyMachines cannot both be true")

	cluster = validCluster()
	cluster.Spec.Kubernetes.Manifests = []omniv1alpha1.KubernetesManifest{{
		Name: "apps",
		Mode: "full",
	}}
	_, err = validator.ValidateCreate(context.Background(), cluster)
	requireErrorContains(t, err, "manifest requires exactly one of file or inline")

	cluster = validCluster()
	cluster.Spec.KernelArgs = []string{"talos.dashboard.disabled=1"}
	warnings, err := validator.ValidateCreate(context.Background(), cluster)
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v, want nil", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "static machines") {
		t.Fatalf("warnings = %#v, want static machine warning", warnings)
	}
}

func TestMachineSetValidation(t *testing.T) {
	t.Parallel()

	controlPlaneValidator := &OmniControlPlaneCustomValidator{}
	controlPlane := &omniv1alpha1.OmniControlPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cp"},
		Spec: omniv1alpha1.OmniControlPlaneSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: "edge"},
			MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
				MachineClass: &omniv1alpha1.MachineClass{Name: "cp", Size: intstr.FromInt32(0)},
			},
		},
	}
	_, err := controlPlaneValidator.ValidateCreate(context.Background(), controlPlane)
	requireErrorContains(t, err, "machineClass size must be greater than zero")

	controlPlane.Spec.MachineClass.Size = intstr.FromString("everything")
	_, err = controlPlaneValidator.ValidateCreate(context.Background(), controlPlane)
	requireErrorContains(t, err, "supported values")

	controlPlane.Spec.MachineClass.Size = intstr.FromString("unlimited")
	controlPlane.Spec.KernelArgs = []string{"talos.dashboard.disabled=1"}
	_, err = controlPlaneValidator.ValidateCreate(context.Background(), controlPlane)
	requireErrorContains(t, err, "kernelArgs are supported only for static machine sets")

	controlPlane.Spec.MachineClass = nil
	controlPlane.Spec.KernelArgs = nil
	controlPlane.Spec.Machines = []string{
		"11111111-1111-4111-8111-111111111111",
		"11111111-1111-4111-8111-111111111111",
	}
	_, err = controlPlaneValidator.ValidateCreate(context.Background(), controlPlane)
	requireErrorContains(t, err, "Duplicate value")

	workersValidator := &OmniWorkersCustomValidator{}
	workers := &omniv1alpha1.OmniWorkers{
		ObjectMeta: metav1.ObjectMeta{Name: "workers"},
		Spec: omniv1alpha1.OmniWorkersSpec{
			ClusterRef:     omniv1alpha1.OmniClusterRef{Name: "edge"},
			WorkerSetName:  "control-planes",
			UpdateStrategy: &omniv1alpha1.UpdateStrategy{Type: "Rolling"},
			MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
				MachineClass: &omniv1alpha1.MachineClass{Name: "worker", Size: intstr.FromString("unlimited")},
			},
		},
	}
	_, err = workersValidator.ValidateCreate(context.Background(), workers)
	requireErrorContains(t, err, "workerSetName is reserved by Omni")
}

func TestMachineValidation(t *testing.T) {
	t.Parallel()

	validator := &OmniMachineCustomValidator{}
	machine := &omniv1alpha1.OmniMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "friendly-name"},
		Spec: omniv1alpha1.OmniMachineSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: "edge"},
		},
	}

	_, err := validator.ValidateCreate(context.Background(), machine)
	requireErrorContains(t, err, "machineID must be a UUID")

	machine.Spec.MachineID = "22222222-2222-4222-8222-222222222222"
	machine.Spec.Patches = []omniv1alpha1.Patch{{File: "patch.yaml", Inline: &apiextensionsv1.JSON{Raw: []byte(`{"machine": {}}`)}}}
	_, err = validator.ValidateCreate(context.Background(), machine)
	requireErrorContains(t, err, "patch must not set both file and inline")

	machine.Spec.Patches = []omniv1alpha1.Patch{{Inline: &apiextensionsv1.JSON{Raw: []byte(`{"machine": {}}`)}}}
	_, err = validator.ValidateCreate(context.Background(), machine)
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v, want nil", err)
	}
}

func TestConnectionWarnings(t *testing.T) {
	t.Parallel()

	validator := &OmniConnectionCustomValidator{}
	connection := &omniv1alpha1.OmniConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "omni"},
		Spec: omniv1alpha1.OmniConnectionSpec{
			Endpoint:              "https://omni.example.test",
			InsecureSkipTLSVerify: true,
			Auth: omniv1alpha1.OmniAuthSpec{
				ServiceAccountSecretRef: omniv1alpha1.SecretKeySelector{Name: "omni-sa", Key: "serviceAccountKey"},
			},
		},
	}

	warnings, err := validator.ValidateCreate(context.Background(), connection)
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v, want nil", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "TLS certificate verification") {
		t.Fatalf("warnings = %#v, want TLS warning", warnings)
	}
}

func validCluster() *omniv1alpha1.OmniCluster {
	return &omniv1alpha1.OmniCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "edge"},
		Spec: omniv1alpha1.OmniClusterSpec{
			ConnectionRef: omniv1alpha1.OmniConnectionRef{Name: "omni"},
			Kubernetes: omniv1alpha1.KubernetesSpec{
				Version: "v1.35.0",
				Manifests: []omniv1alpha1.KubernetesManifest{{
					Name:   "apps",
					Mode:   "full",
					Inline: []apiextensionsv1.JSON{{Raw: []byte(`{"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": "apps"}}`)}},
				}},
			},
			Talos: omniv1alpha1.TalosSpec{Version: "v1.13.2"},
			Patches: []omniv1alpha1.Patch{{
				Inline: &apiextensionsv1.JSON{Raw: []byte(`{"machine": {}}`)},
			}},
		},
	}
}

func requireErrorContains(t *testing.T, err error, substring string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want substring %q", substring)
	}
	if !strings.Contains(err.Error(), substring) {
		t.Fatalf("error = %q, want substring %q", err.Error(), substring)
	}
}
