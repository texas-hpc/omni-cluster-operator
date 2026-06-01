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

// OmniClusterAddonHelmSpec describes one Helm chart rendered into an Omni manifest.
type OmniClusterAddonHelmSpec struct {
	// repository is the Helm repository URL that serves the chart.
	// +required
	// +kubebuilder:validation:MinLength=1
	Repository string `json:"repository"`

	// chart is the Helm chart name to render.
	// +required
	// +kubebuilder:validation:MinLength=1
	Chart string `json:"chart"`

	// version is the Helm chart version to render.
	// +required
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// releaseName is the Helm release name used while rendering.
	// Defaults to the OmniClusterAddon metadata.name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ReleaseName string `json:"releaseName,omitempty"`

	// namespace is the Kubernetes namespace for rendered objects.
	// +kubebuilder:default:=default
	// +optional
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace,omitempty"`

	// values is passed to Helm while rendering the chart.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`
}

// OmniClusterAddonSpec defines a generic Helm-rendered addon for an Omni cluster template.
type OmniClusterAddonSpec struct {
	// clusterRef attaches this addon to one OmniCluster in the same namespace.
	// +required
	ClusterRef OmniClusterRef `json:"clusterRef"`

	// manifestName is the Omni manifest sync entry name.
	// Defaults to the OmniClusterAddon metadata.name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ManifestName string `json:"manifestName,omitempty"`

	// mode controls how Omni applies the rendered manifest.
	// +kubebuilder:validation:Enum=one-time;full
	// +kubebuilder:default:=full
	// +optional
	Mode string `json:"mode,omitempty"`

	// helm configures the chart to render into the Omni template.
	// +required
	Helm OmniClusterAddonHelmSpec `json:"helm"`
}

// OmniClusterAddonStatus defines the observed state of OmniClusterAddon.
type OmniClusterAddonStatus struct {
	CommonStatusFields `json:",inline"`

	// clusterRef is the last cluster reference observed by the controller.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`

	// chart is the last Helm chart rendered by the controller.
	// +optional
	Chart string `json:"chart,omitempty"`

	// chartVersion is the last Helm chart version rendered by the controller.
	// +optional
	ChartVersion string `json:"chartVersion,omitempty"`

	// manifestName is the Omni manifest sync name rendered by the controller.
	// +optional
	ManifestName string `json:"manifestName,omitempty"`

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
// +kubebuilder:resource:path=omniclusteraddons,singular=omniclusteraddon

// OmniClusterAddon is the Schema for the omniclusteraddons API.
type OmniClusterAddon struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniClusterAddon.
	// +required
	Spec OmniClusterAddonSpec `json:"spec"`

	// status defines the observed state of OmniClusterAddon.
	// +optional
	Status OmniClusterAddonStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniClusterAddonList contains a list of OmniClusterAddon.
type OmniClusterAddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniClusterAddon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniClusterAddon{}, &OmniClusterAddonList{})
}
