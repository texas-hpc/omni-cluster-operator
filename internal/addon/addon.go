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

package addon

import (
	"bytes"
	"encoding/json"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/renderedmanifest"
)

const (
	DefaultNamespace             = "default"
	DefaultMode                  = "full"
	RenderedManifestSecretKey    = renderedmanifest.SecretKey
	RenderedManifestHashKey      = renderedmanifest.HashAnnotation
	RenderedManifestSpecHashKey  = "omni.texashpc.com/addon-spec-hash"
	RenderedManifestOwnerLabel   = "omni.texashpc.com/omniclusteraddon"
	RenderedManifestClusterLabel = "omni.texashpc.com/cluster"
)

const specHashVersion = "v1"

// ReleaseName returns the normalized Helm release name.
func ReleaseName(item *omniv1alpha1.OmniClusterAddon) string {
	if item.Spec.Helm.ReleaseName != "" {
		return item.Spec.Helm.ReleaseName
	}

	return item.Name
}

// Namespace returns the normalized target namespace.
func Namespace(item *omniv1alpha1.OmniClusterAddon) string {
	if item.Spec.Helm.Namespace != "" {
		return item.Spec.Helm.Namespace
	}

	return DefaultNamespace
}

// ManifestName returns the normalized Omni manifest sync name.
func ManifestName(item *omniv1alpha1.OmniClusterAddon) string {
	if item.Spec.ManifestName != "" {
		return item.Spec.ManifestName
	}

	return item.Name
}

// Mode returns the normalized Omni manifest sync mode.
func Mode(item *omniv1alpha1.OmniClusterAddon) string {
	if item.Spec.Mode != "" {
		return item.Spec.Mode
	}

	return DefaultMode
}

// RenderedManifestSecretName returns the Secret name used to cache Helm output.
func RenderedManifestSecretName(item *omniv1alpha1.OmniClusterAddon) string {
	return fmt.Sprintf("%s-addon-manifest", item.Name)
}

// RenderedManifestLabels returns labels used to discover cached addon manifests.
func RenderedManifestLabels(item *omniv1alpha1.OmniClusterAddon) map[string]string {
	return map[string]string{
		RenderedManifestOwnerLabel:   item.Name,
		RenderedManifestClusterLabel: item.Spec.ClusterRef.Name,
	}
}

// RenderedManifestHash returns a SHA-256 hash for rendered manifest bytes.
func RenderedManifestHash(manifest []byte) string {
	return renderedmanifest.Hash(manifest)
}

// SpecHash returns a stable hash of addon inputs that affect rendered output.
func SpecHash(item *omniv1alpha1.OmniClusterAddon) (string, error) {
	values, err := Values(item)
	if err != nil {
		return "", err
	}

	normalized := struct {
		Version      string         `json:"version"`
		Repository   string         `json:"repository"`
		Chart        string         `json:"chart"`
		ChartVersion string         `json:"chartVersion"`
		ReleaseName  string         `json:"releaseName"`
		Namespace    string         `json:"namespace"`
		ManifestName string         `json:"manifestName"`
		Mode         string         `json:"mode"`
		Values       map[string]any `json:"values"`
	}{
		Version:      specHashVersion,
		Repository:   item.Spec.Helm.Repository,
		Chart:        item.Spec.Helm.Chart,
		ChartVersion: item.Spec.Helm.Version,
		ReleaseName:  ReleaseName(item),
		Namespace:    Namespace(item),
		ManifestName: ManifestName(item),
		Mode:         Mode(item),
		Values:       values,
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal addon spec hash payload: %w", err)
	}

	return renderedmanifest.Hash(payload), nil
}

// Values returns decoded Helm values for the addon.
func Values(item *omniv1alpha1.OmniClusterAddon) (map[string]any, error) {
	return decodeValues(item.Spec.Helm.Values)
}

// ParseRenderedManifest converts a rendered multi-document YAML manifest into Omni inline JSON objects.
func ParseRenderedManifest(manifest []byte) ([]apiextensionsv1.JSON, error) {
	return renderedmanifest.Parse(manifest, renderedmanifest.AllowEmpty)
}

// SecretHasCurrentManifest reports whether a Secret already contains the desired render.
func SecretHasCurrentManifest(secret client.Object, data map[string][]byte, specHash string) bool {
	return renderedmanifest.SecretHasCurrentManifest(secret, data, RenderedManifestSpecHashKey, specHash)
}

func decodeValues(values *apiextensionsv1.JSON) (map[string]any, error) {
	if values == nil || len(bytes.TrimSpace(values.Raw)) == 0 {
		return map[string]any{}, nil
	}

	var decoded any
	if err := json.Unmarshal(values.Raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode addon values: %w", err)
	}

	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("addon values must be a JSON object")
	}

	return object, nil
}
