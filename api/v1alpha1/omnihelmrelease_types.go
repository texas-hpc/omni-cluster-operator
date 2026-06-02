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

const (
	// HelmReleaseDeletionPolicyUninstall uninstalls the workload-cluster Helm release when the CR is deleted.
	HelmReleaseDeletionPolicyUninstall HelmReleaseDeletionPolicy = "Uninstall"
	// HelmReleaseDeletionPolicyOrphan leaves the workload-cluster Helm release when the CR is deleted.
	HelmReleaseDeletionPolicyOrphan HelmReleaseDeletionPolicy = "Orphan"
)

// HelmReleaseDeletionPolicy controls workload-cluster Helm release cleanup on deletion.
// +kubebuilder:validation:Enum=Uninstall;Orphan
type HelmReleaseDeletionPolicy string

// HelmReleaseKubeconfigSecretRef identifies the Secret key that contains workload-cluster kubeconfig data.
type HelmReleaseKubeconfigSecretRef struct {
	// name is the kubeconfig Secret name in the same namespace.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// key is the Secret data key. Defaults to kubeconfig.
	// +kubebuilder:default:=kubeconfig
	// +optional
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key,omitempty"`
}

// OmniHelmChartSpec describes one Helm chart reconciled directly into a workload cluster.
type OmniHelmChartSpec struct {
	// repository is the Helm repository URL that serves the chart.
	// +required
	// +kubebuilder:validation:MinLength=1
	Repository string `json:"repository"`

	// chart is the Helm chart name to reconcile.
	// +required
	// +kubebuilder:validation:MinLength=1
	Chart string `json:"chart"`

	// version is the Helm chart version to reconcile.
	// +required
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// values is passed to Helm during install and upgrade.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`
}

// OmniHelmReleaseSpec defines a direct workload-cluster Helm release.
type OmniHelmReleaseSpec struct {
	// clusterRef attaches this release to one OmniCluster in the same namespace.
	// +required
	ClusterRef OmniClusterRef `json:"clusterRef"`

	// kubeconfigSecretRef selects a Secret key containing a workload-cluster kubeconfig.
	// Create this explicitly, for example with OmniKubeconfigExport.
	// +required
	KubeconfigSecretRef HelmReleaseKubeconfigSecretRef `json:"kubeconfigSecretRef"`

	// releaseName is the Helm release name. Defaults to the OmniHelmRelease metadata.name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ReleaseName string `json:"releaseName,omitempty"`

	// namespace is the workload-cluster namespace for the Helm release.
	// +kubebuilder:default:=default
	// +optional
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace,omitempty"`

	// chart configures the Helm chart to install or upgrade.
	// +required
	Chart OmniHelmChartSpec `json:"chart"`

	// createNamespace asks Helm to create the release namespace during install.
	// +optional
	CreateNamespace bool `json:"createNamespace,omitempty"`

	// wait asks Helm to wait for reconciled resources to become ready.
	// Hooks are waited for even when wait is false.
	// +optional
	Wait bool `json:"wait,omitempty"`

	// waitForJobs includes Jobs in Helm's wait behavior.
	// +optional
	WaitForJobs bool `json:"waitForJobs,omitempty"`

	// timeout bounds Helm install, upgrade, wait, hook, rollback, and uninstall operations.
	// Defaults to 5m.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// atomic rolls back failed upgrades and uninstalls failed installs.
	// +optional
	Atomic bool `json:"atomic,omitempty"`

	// disableHooks prevents Helm hooks from running during install, upgrade, and uninstall.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// skipCRDs skips CRDs on install. Helm does not upgrade CRDs.
	// +optional
	SkipCRDs bool `json:"skipCRDs,omitempty"`

	// maxHistory limits the number of Helm release revisions retained. Zero uses Helm's default.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxHistory int `json:"maxHistory,omitempty"`

	// deletionPolicy controls whether the workload-cluster Helm release is uninstalled when this CR is deleted.
	// +kubebuilder:default:=Uninstall
	// +optional
	DeletionPolicy HelmReleaseDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// OmniHelmReleaseStatus defines the observed state of OmniHelmRelease.
type OmniHelmReleaseStatus struct {
	CommonStatusFields `json:",inline"`

	// clusterRef is the last cluster reference observed by the controller.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`

	// kubeconfigSecretRef is the kubeconfig Secret name observed by the controller.
	// +optional
	KubeconfigSecretRef string `json:"kubeconfigSecretRef,omitempty"`

	// kubeconfigSecretKey is the kubeconfig Secret key observed by the controller.
	// +optional
	KubeconfigSecretKey string `json:"kubeconfigSecretKey,omitempty"`

	// releaseName is the Helm release name observed by the controller.
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// namespace is the workload-cluster namespace observed by the controller.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// chart is the Helm chart name observed by the controller.
	// +optional
	Chart string `json:"chart,omitempty"`

	// chartVersion is the Helm chart version observed by the controller.
	// +optional
	ChartVersion string `json:"chartVersion,omitempty"`

	// releaseRevision is the last Helm release revision observed by the controller.
	// +optional
	ReleaseRevision int64 `json:"releaseRevision,omitempty"`

	// releaseStatus is the last Helm release status observed by the controller.
	// +optional
	ReleaseStatus string `json:"releaseStatus,omitempty"`

	// lastAction is the last Helm action attempted by the controller.
	// +optional
	LastAction string `json:"lastAction,omitempty"`

	// lastAttemptTime is when the controller last attempted a Helm action.
	// +optional
	LastAttemptTime *metav1.Time `json:"lastAttemptTime,omitempty"`

	// lastSuccessTime is when the controller last completed a Helm action successfully.
	// +optional
	LastSuccessTime *metav1.Time `json:"lastSuccessTime,omitempty"`

	// lastError is the last Helm or credential error observed by the controller.
	// +optional
	LastError string `json:"lastError,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=omnihelmreleases,singular=omnihelmrelease

// OmniHelmRelease is the Schema for the omnihelmreleases API.
type OmniHelmRelease struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniHelmRelease.
	// +required
	Spec OmniHelmReleaseSpec `json:"spec"`

	// status defines the observed state of OmniHelmRelease.
	// +optional
	Status OmniHelmReleaseStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniHelmReleaseList contains a list of OmniHelmRelease.
type OmniHelmReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniHelmRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniHelmRelease{}, &OmniHelmReleaseList{})
}
