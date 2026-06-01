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

package omnitemplate

import (
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const (
	staticClusterName       = "edge"
	machineClassClusterName = "elastic"
	workersName             = "workers"
	testKubernetesVersion   = "v1.35.0"
	testTalosVersion        = "v1.13.2"
	testCiliumManifestName  = "cilium"
	testManifestMode        = "full"
)

func TestRenderAndValidateStaticTemplate(t *testing.T) {
	t.Parallel()

	maxParallelism := int32(2)
	controlPlaneID := "11111111-1111-4111-8111-111111111111"
	workerID := "22222222-2222-4222-8222-222222222222"
	result, err := Render(Inputs{
		Cluster: &omniv1alpha1.OmniCluster{
			ObjectMeta: metav1.ObjectMeta{Name: staticClusterName},
			Spec: omniv1alpha1.OmniClusterSpec{
				Kubernetes: omniv1alpha1.KubernetesSpec{
					Version: testKubernetesVersion,
					Manifests: []omniv1alpha1.KubernetesManifest{{
						Name: "namespace",
						Mode: testManifestMode,
						Inline: []apiextensionsv1.JSON{{
							Raw: []byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"apps"}}`),
						}},
					}},
				},
				Talos: omniv1alpha1.TalosSpec{Version: testTalosVersion},
				Features: &omniv1alpha1.ClusterFeatures{
					EnableWorkloadProxy: true,
				},
				Patches: []omniv1alpha1.Patch{{
					Name:   "cluster-network",
					Inline: &apiextensionsv1.JSON{Raw: []byte(`{"cluster":{"network":{"cni":{"name":"none"}}}}`)},
				}},
			},
		},
		ControlPlane: &omniv1alpha1.OmniControlPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "edge-control-plane"},
			Spec: omniv1alpha1.OmniControlPlaneSpec{
				ClusterRef: omniv1alpha1.OmniClusterRef{Name: staticClusterName},
				MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
					Machines: []string{controlPlaneID},
				},
			},
		},
		Workers: []omniv1alpha1.OmniWorkers{{
			ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
			Spec: omniv1alpha1.OmniWorkersSpec{
				ClusterRef: omniv1alpha1.OmniClusterRef{Name: staticClusterName},
				MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
					Machines: []string{workerID},
				},
				UpdateStrategy: &omniv1alpha1.UpdateStrategy{
					Type:    "Rolling",
					Rolling: &omniv1alpha1.RollingStrategy{MaxParallelism: &maxParallelism},
				},
			},
		}},
		Machines: []omniv1alpha1.OmniMachine{
			machine(controlPlaneID, staticClusterName),
			machine(workerID, staticClusterName),
		},
		Cilium: &CiliumInput{
			ResourceName:         "edge-cilium",
			ManifestName:         testCiliumManifestName,
			Mode:                 testManifestMode,
			KubeProxyReplacement: true,
			Manifest: []apiextensionsv1.JSON{{
				Raw: []byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system"}}`),
			}},
		},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	rendered := string(result.Template)
	for _, want := range []string{
		"kind: Cluster",
		"name: edge",
		"kind: ControlPlane",
		"kind: Workers",
		"name: gpu",
		"kind: Machine",
		"maxParallelism: 2",
		"kind: Namespace",
		"disable-default-cni-for-cilium",
		"proxy:",
		"disabled: true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered template missing %q:\n%s", want, rendered)
		}
	}

	if result.TemplateHash == "" {
		t.Fatal("TemplateHash is empty")
	}

	if err := Validate(result.Template, ""); err != nil {
		t.Fatalf("Validate() error = %v\n%s", err, rendered)
	}
}

func TestRenderAndValidateMachineClassTemplate(t *testing.T) {
	t.Parallel()

	result, err := Render(Inputs{
		Cluster: &omniv1alpha1.OmniCluster{
			ObjectMeta: metav1.ObjectMeta{Name: machineClassClusterName},
			Spec: omniv1alpha1.OmniClusterSpec{
				Kubernetes: omniv1alpha1.KubernetesSpec{Version: testKubernetesVersion},
				Talos:      omniv1alpha1.TalosSpec{Version: testTalosVersion},
			},
		},
		ControlPlane: &omniv1alpha1.OmniControlPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "cp"},
			Spec: omniv1alpha1.OmniControlPlaneSpec{
				ClusterRef: omniv1alpha1.OmniClusterRef{Name: machineClassClusterName},
				MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
					MachineClass: &omniv1alpha1.MachineClass{
						Name: "control-plane",
						Size: intstr.FromInt32(3),
					},
				},
			},
		},
		Workers: []omniv1alpha1.OmniWorkers{{
			ObjectMeta: metav1.ObjectMeta{Name: workersName},
			Spec: omniv1alpha1.OmniWorkersSpec{
				ClusterRef: omniv1alpha1.OmniClusterRef{Name: machineClassClusterName},
				MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
					MachineClass: &omniv1alpha1.MachineClass{
						Name: "gpu-workers",
						Size: intstr.FromString("unlimited"),
					},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	rendered := string(result.Template)
	if !strings.Contains(rendered, "size: unlimited") {
		t.Fatalf("rendered template missing string machineClass size:\n%s", rendered)
	}

	if err := Validate(result.Template, ""); err != nil {
		t.Fatalf("Validate() error = %v\n%s", err, rendered)
	}
}

func TestRenderAndValidateMultipleWorkerSets(t *testing.T) {
	t.Parallel()

	result, err := Render(Inputs{
		Cluster: &omniv1alpha1.OmniCluster{
			ObjectMeta: metav1.ObjectMeta{Name: machineClassClusterName},
			Spec: omniv1alpha1.OmniClusterSpec{
				Kubernetes: omniv1alpha1.KubernetesSpec{Version: testKubernetesVersion},
				Talos:      omniv1alpha1.TalosSpec{Version: testTalosVersion},
			},
		},
		ControlPlane: &omniv1alpha1.OmniControlPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "cp"},
			Spec: omniv1alpha1.OmniControlPlaneSpec{
				ClusterRef: omniv1alpha1.OmniClusterRef{Name: machineClassClusterName},
				MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
					MachineClass: &omniv1alpha1.MachineClass{
						Name: "control-plane",
						Size: intstr.FromInt32(3),
					},
				},
			},
		},
		Workers: []omniv1alpha1.OmniWorkers{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "gpu-x86"},
				Spec: omniv1alpha1.OmniWorkersSpec{
					ClusterRef:    omniv1alpha1.OmniClusterRef{Name: machineClassClusterName},
					WorkerSetName: "gpu-x86-oss",
					MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
						MachineClass: &omniv1alpha1.MachineClass{
							Name: "gpu-x86",
							Size: intstr.FromInt32(2),
						},
						SystemExtensions: []string{
							"siderolabs/nvidia-open-gpu-kernel-modules-production",
							"siderolabs/nvidia-container-toolkit-production",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "gpu-arm64"},
				Spec: omniv1alpha1.OmniWorkersSpec{
					ClusterRef:    omniv1alpha1.OmniClusterRef{Name: machineClassClusterName},
					WorkerSetName: "gpu-arm64-proprietary",
					MachineSetSpecFields: omniv1alpha1.MachineSetSpecFields{
						MachineClass: &omniv1alpha1.MachineClass{
							Name: "gpu-arm64",
							Size: intstr.FromInt32(1),
						},
						SystemExtensions: []string{
							"siderolabs/nonfree-kmod-nvidia-production",
							"siderolabs/nvidia-container-toolkit-production",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	rendered := string(result.Template)
	for _, want := range []string{
		"name: gpu-arm64-proprietary",
		"name: gpu-x86-oss",
		"name: gpu-arm64",
		"name: gpu-x86",
		"siderolabs/nonfree-kmod-nvidia-production",
		"siderolabs/nvidia-open-gpu-kernel-modules-production",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered template missing %q:\n%s", want, rendered)
		}
	}
	if got := strings.Count(rendered, "kind: Workers"); got != 2 {
		t.Fatalf("rendered Workers document count = %d, want 2:\n%s", got, rendered)
	}
	if got, want := result.WorkersRefs, []string{"gpu-arm64", "gpu-x86"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("WorkersRefs = %#v, want %#v", got, want)
	}

	if err := Validate(result.Template, ""); err != nil {
		t.Fatalf("Validate() error = %v\n%s", err, rendered)
	}
}

func TestRenderRejectsDuplicateCiliumManifestName(t *testing.T) {
	t.Parallel()

	_, err := Render(Inputs{
		Cluster: &omniv1alpha1.OmniCluster{
			ObjectMeta: metav1.ObjectMeta{Name: staticClusterName},
			Spec: omniv1alpha1.OmniClusterSpec{
				Kubernetes: omniv1alpha1.KubernetesSpec{
					Version: testKubernetesVersion,
					Manifests: []omniv1alpha1.KubernetesManifest{{
						Name: testCiliumManifestName,
						Mode: testManifestMode,
						Inline: []apiextensionsv1.JSON{{
							Raw: []byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"existing"}}`),
						}},
					}},
				},
				Talos: omniv1alpha1.TalosSpec{Version: testTalosVersion},
			},
		},
		ControlPlane: &omniv1alpha1.OmniControlPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "edge-control-plane"},
			Spec: omniv1alpha1.OmniControlPlaneSpec{
				ClusterRef: omniv1alpha1.OmniClusterRef{Name: staticClusterName},
			},
		},
		Cilium: &CiliumInput{
			ResourceName: "edge-cilium",
			ManifestName: testCiliumManifestName,
			Manifest: []apiextensionsv1.JSON{{
				Raw: []byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system"}}`),
			}},
		},
	})
	if err == nil {
		t.Fatal("Render() error = nil, want duplicate manifest name error")
	}
	if !strings.Contains(err.Error(), `duplicate OmniCluster.spec.kubernetes.manifests[].name "cilium"`) {
		t.Fatalf("Render() error = %v, want duplicate cilium manifest name", err)
	}
}

func machine(id, cluster string) omniv1alpha1.OmniMachine {
	return omniv1alpha1.OmniMachine{
		ObjectMeta: metav1.ObjectMeta{Name: id},
		Spec: omniv1alpha1.OmniMachineSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: cluster},
			MachineID:  id,
		},
	}
}
