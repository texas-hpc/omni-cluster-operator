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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OmniCiliumSpec defines a Cilium installation rendered into an Omni cluster template.
type OmniCiliumSpec struct {
	// clusterRef attaches this Cilium installation to one OmniCluster in the same namespace.
	// +required
	ClusterRef OmniClusterRef `json:"clusterRef"`

	// chartVersion is the Cilium Helm chart version to render.
	// +required
	// +kubebuilder:validation:MinLength=1
	ChartVersion string `json:"chartVersion"`

	// chartRepository is the Helm repository URL that serves the Cilium chart.
	// +kubebuilder:default:="https://helm.cilium.io/"
	// +optional
	ChartRepository string `json:"chartRepository,omitempty"`

	// releaseName is the Helm release name used while rendering Cilium.
	// +kubebuilder:default:=cilium
	// +optional
	// +kubebuilder:validation:MinLength=1
	ReleaseName string `json:"releaseName,omitempty"`

	// namespace is the Kubernetes namespace for the rendered Cilium objects.
	// +kubebuilder:default:="kube-system"
	// +optional
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace,omitempty"`

	// manifestName is the Omni manifest sync entry name.
	// +kubebuilder:default:=cilium
	// +optional
	// +kubebuilder:validation:MinLength=1
	ManifestName string `json:"manifestName,omitempty"`

	// mode controls how Omni applies the rendered Cilium manifest.
	// +kubebuilder:validation:Enum=one-time;full
	// +kubebuilder:default:=full
	// +optional
	Mode string `json:"mode,omitempty"`

	// values is merged over the operator's Talos-compatible Cilium defaults before rendering.
	// Set kubeProxyReplacement: true here to also disable kube-proxy in the generated Talos patch.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`
}

// OmniCiliumStatus defines the observed state of OmniCilium.
type OmniCiliumStatus struct {
	CommonStatusFields `json:",inline"`

	// clusterRef is the last cluster reference observed by the controller.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`

	// chartVersion is the last Cilium chart version rendered by the controller.
	// +optional
	ChartVersion string `json:"chartVersion,omitempty"`

	// manifestName is the Omni manifest sync name rendered by the controller.
	// +optional
	ManifestName string `json:"manifestName,omitempty"`

	// kubeProxyReplacement records whether the rendered values request Cilium kube-proxy replacement.
	// +optional
	KubeProxyReplacement bool `json:"kubeProxyReplacement,omitempty"`

	// renderedManifestSecretRef names the Secret containing the cached rendered manifest.
	// +optional
	RenderedManifestSecretRef string `json:"renderedManifestSecretRef,omitempty"`

	// renderedManifestHash is the SHA-256 hash of the cached rendered manifest.
	// +optional
	RenderedManifestHash string `json:"renderedManifestHash,omitempty"`

	// lastRenderTime is when the controller last rendered or reused the cached manifest.
	// +optional
	LastRenderTime *metav1.Time `json:"lastRenderTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=omniciliums,singular=omnicilium

// OmniCilium is the Schema for the omniciliums API
type OmniCilium struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniCilium
	// +required
	Spec OmniCiliumSpec `json:"spec"`

	// status defines the observed state of OmniCilium
	// +optional
	Status OmniCiliumStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniCiliumList contains a list of OmniCilium
type OmniCiliumList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniCilium `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniCilium{}, &OmniCiliumList{})
}
