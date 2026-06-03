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

package helmrelease

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const (
	DefaultNamespace      = "default"
	DefaultKubeconfigKey  = "kubeconfig"
	DefaultTimeout        = 5 * time.Minute
	DefaultDeletionPolicy = omniv1alpha1.HelmReleaseDeletionPolicyUninstall
	ActionInstall         = "Install"
	ActionUpgrade         = "Upgrade"
	ActionUninstall       = "Uninstall"
	StatusDeployed        = "deployed"
	StatusUninstalled     = "uninstalled"
	StatusUnknown         = "unknown"
)

// Result describes the release state returned by Helm.
type Result struct {
	Action       string
	ReleaseName  string
	Namespace    string
	Chart        string
	ChartVersion string
	Revision     int
	Status       string
}

// ReleaseName returns the normalized Helm release name.
func ReleaseName(item *omniv1alpha1.OmniHelmRelease) string {
	if item.Spec.ReleaseName != "" {
		return item.Spec.ReleaseName
	}

	return item.Name
}

// Namespace returns the normalized Helm release namespace.
func Namespace(item *omniv1alpha1.OmniHelmRelease) string {
	if item.Spec.Namespace != "" {
		return item.Spec.Namespace
	}

	return DefaultNamespace
}

// KubeconfigSecretKey returns the normalized kubeconfig Secret data key.
func KubeconfigSecretKey(item *omniv1alpha1.OmniHelmRelease) string {
	if item.Spec.KubeconfigSecretRef.Key != "" {
		return item.Spec.KubeconfigSecretRef.Key
	}

	return DefaultKubeconfigKey
}

// Timeout returns the normalized Helm action timeout.
func Timeout(item *omniv1alpha1.OmniHelmRelease) time.Duration {
	if item.Spec.Timeout != nil && item.Spec.Timeout.Duration > 0 {
		return item.Spec.Timeout.Duration
	}

	return DefaultTimeout
}

// DeletionPolicy returns the normalized deletion policy.
func DeletionPolicy(item *omniv1alpha1.OmniHelmRelease) omniv1alpha1.HelmReleaseDeletionPolicy {
	if item.Spec.DeletionPolicy != "" {
		return item.Spec.DeletionPolicy
	}

	return DefaultDeletionPolicy
}

// Values returns decoded Helm values for the release.
func Values(item *omniv1alpha1.OmniHelmRelease) (map[string]any, error) {
	values := item.Spec.Chart.Values
	if values == nil || len(bytes.TrimSpace(values.Raw)) == 0 {
		return map[string]any{}, nil
	}

	var decoded any
	if err := json.Unmarshal(values.Raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode Helm release values: %w", err)
	}

	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("helm release values must be a JSON object")
	}

	return object, nil
}
