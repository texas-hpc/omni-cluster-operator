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

package cilium

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigsyaml "sigs.k8s.io/yaml"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const (
	ChartName                    = "cilium"
	DefaultChartRepository       = "https://helm.cilium.io/"
	DefaultReleaseName           = "cilium"
	DefaultNamespace             = "kube-system"
	DefaultManifestName          = "cilium"
	DefaultMode                  = "full"
	RenderedManifestSecretKey    = "manifest.yaml"
	RenderedManifestHashKey      = "omni.texas-hpc.org/rendered-manifest-hash"
	RenderedManifestSpecHashKey  = "omni.texas-hpc.org/cilium-spec-hash"
	RenderedManifestOwnerLabel   = "omni.texas-hpc.org/omnicilium"
	RenderedManifestClusterLabel = "omni.texas-hpc.org/cluster"
)

const specHashVersion = "v1"

// ChartRepository returns the normalized chart repository URL.
func ChartRepository(install *omniv1alpha1.OmniCilium) string {
	if install.Spec.ChartRepository != "" {
		return install.Spec.ChartRepository
	}

	return DefaultChartRepository
}

// ReleaseName returns the normalized Helm release name.
func ReleaseName(install *omniv1alpha1.OmniCilium) string {
	if install.Spec.ReleaseName != "" {
		return install.Spec.ReleaseName
	}

	return DefaultReleaseName
}

// Namespace returns the normalized target namespace.
func Namespace(install *omniv1alpha1.OmniCilium) string {
	if install.Spec.Namespace != "" {
		return install.Spec.Namespace
	}

	return DefaultNamespace
}

// ManifestName returns the normalized Omni manifest sync name.
func ManifestName(install *omniv1alpha1.OmniCilium) string {
	if install.Spec.ManifestName != "" {
		return install.Spec.ManifestName
	}

	return DefaultManifestName
}

// Mode returns the normalized Omni manifest sync mode.
func Mode(install *omniv1alpha1.OmniCilium) string {
	if install.Spec.Mode != "" {
		return install.Spec.Mode
	}

	return DefaultMode
}

// RenderedManifestSecretName returns the Secret name used to cache Helm output.
func RenderedManifestSecretName(install *omniv1alpha1.OmniCilium) string {
	return fmt.Sprintf("%s-cilium-manifest", install.Name)
}

// RenderedManifestLabels returns labels used to discover cached Cilium manifests.
func RenderedManifestLabels(install *omniv1alpha1.OmniCilium) map[string]string {
	return map[string]string{
		RenderedManifestOwnerLabel:   install.Name,
		RenderedManifestClusterLabel: install.Spec.ClusterRef.Name,
	}
}

// RenderedManifestHash returns a SHA-256 hash for rendered manifest bytes.
func RenderedManifestHash(manifest []byte) string {
	sum := sha256.Sum256(manifest)
	return hex.EncodeToString(sum[:])
}

// SpecHash returns a stable hash of Cilium inputs that affect rendered output.
func SpecHash(install *omniv1alpha1.OmniCilium) (string, error) {
	values, _, err := Values(install)
	if err != nil {
		return "", err
	}

	normalized := struct {
		Version         string         `json:"version"`
		ChartVersion    string         `json:"chartVersion"`
		ChartRepository string         `json:"chartRepository"`
		ReleaseName     string         `json:"releaseName"`
		Namespace       string         `json:"namespace"`
		ManifestName    string         `json:"manifestName"`
		Mode            string         `json:"mode"`
		Values          map[string]any `json:"values"`
	}{
		Version:         specHashVersion,
		ChartVersion:    install.Spec.ChartVersion,
		ChartRepository: ChartRepository(install),
		ReleaseName:     ReleaseName(install),
		Namespace:       Namespace(install),
		ManifestName:    ManifestName(install),
		Mode:            Mode(install),
		Values:          values,
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal cilium spec hash payload: %w", err)
	}

	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// Values returns Talos-compatible Cilium Helm values merged with user overrides.
func Values(install *omniv1alpha1.OmniCilium) (map[string]any, bool, error) {
	values := talosDefaultValues()
	overrides, err := decodeValues(install.Spec.Values)
	if err != nil {
		return nil, false, err
	}
	mergeMaps(values, overrides)

	kubeProxyReplacement, err := boolValue(values, "kubeProxyReplacement")
	if err != nil {
		return nil, false, err
	}
	if kubeProxyReplacement {
		setDefault(values, "k8sServiceHost", "localhost")
		setDefault(values, "k8sServicePort", 7445)
	}

	return values, kubeProxyReplacement, nil
}

// ParseRenderedManifest converts a rendered multi-document YAML manifest into Omni inline JSON objects.
func ParseRenderedManifest(manifest []byte) ([]apiextensionsv1.JSON, error) {
	reader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(manifest)))
	var objects []apiextensionsv1.JSON

	for {
		doc, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}

			return nil, fmt.Errorf("read manifest document: %w", err)
		}

		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		rawJSON, err := sigsyaml.YAMLToJSON(doc)
		if err != nil {
			return nil, fmt.Errorf("convert manifest document to JSON: %w", err)
		}
		if string(bytes.TrimSpace(rawJSON)) == "null" {
			continue
		}

		var compact bytes.Buffer
		if err := json.Compact(&compact, rawJSON); err != nil {
			return nil, fmt.Errorf("compact manifest document JSON: %w", err)
		}
		if compact.Len() == 0 || compact.String() == "{}" {
			continue
		}

		objects = append(objects, apiextensionsv1.JSON{Raw: compact.Bytes()})
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("rendered manifest contains no Kubernetes objects")
	}

	return objects, nil
}

// SecretHasCurrentManifest reports whether a Secret already contains the desired render.
func SecretHasCurrentManifest(secret client.Object, data map[string][]byte, specHash string) bool {
	if secret.GetAnnotations()[RenderedManifestSpecHashKey] != specHash {
		return false
	}

	return len(data[RenderedManifestSecretKey]) > 0
}

func talosDefaultValues() map[string]any {
	return map[string]any{
		"ipam": map[string]any{
			"mode": "kubernetes",
		},
		"kubeProxyReplacement": false,
		"securityContext": map[string]any{
			"capabilities": map[string]any{
				"ciliumAgent": []any{
					"CHOWN",
					"KILL",
					"NET_ADMIN",
					"NET_RAW",
					"IPC_LOCK",
					"SYS_ADMIN",
					"SYS_RESOURCE",
					"DAC_OVERRIDE",
					"FOWNER",
					"SETGID",
					"SETUID",
				},
				"cleanCiliumState": []any{
					"NET_ADMIN",
					"SYS_ADMIN",
					"SYS_RESOURCE",
				},
			},
		},
		"cgroup": map[string]any{
			"autoMount": map[string]any{
				"enabled": false,
			},
			"hostRoot": "/sys/fs/cgroup",
		},
	}
}

func decodeValues(values *apiextensionsv1.JSON) (map[string]any, error) {
	if values == nil || len(bytes.TrimSpace(values.Raw)) == 0 {
		return map[string]any{}, nil
	}

	var decoded any
	if err := json.Unmarshal(values.Raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode cilium values: %w", err)
	}

	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cilium values must be a JSON object")
	}

	return object, nil
}

func mergeMaps(dst, src map[string]any) {
	for key, srcValue := range src {
		srcMap, srcIsMap := srcValue.(map[string]any)
		dstMap, dstIsMap := dst[key].(map[string]any)
		if srcIsMap && dstIsMap {
			mergeMaps(dstMap, srcMap)
			continue
		}

		dst[key] = srcValue
	}
}

func boolValue(values map[string]any, key string) (bool, error) {
	value, ok := values[key]
	if !ok {
		return false, nil
	}

	boolValue, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", key)
	}

	return boolValue, nil
}

func setDefault(values map[string]any, key string, value any) {
	if _, ok := values[key]; ok {
		return
	}

	values[key] = value
}
