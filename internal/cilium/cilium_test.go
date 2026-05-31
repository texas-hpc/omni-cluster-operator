package cilium

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const netAdminCapability = "NET_ADMIN"

func TestNormalizedSpecDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	install := &omniv1alpha1.OmniCilium{
		ObjectMeta: metav1.ObjectMeta{Name: "edge-cilium"},
		Spec: omniv1alpha1.OmniCiliumSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: "edge"},
		},
	}

	if got := ChartRepository(install); got != DefaultChartRepository {
		t.Fatalf("ChartRepository() = %q, want %q", got, DefaultChartRepository)
	}
	if got := ReleaseName(install); got != DefaultReleaseName {
		t.Fatalf("ReleaseName() = %q, want %q", got, DefaultReleaseName)
	}
	if got := Namespace(install); got != DefaultNamespace {
		t.Fatalf("Namespace() = %q, want %q", got, DefaultNamespace)
	}
	if got := ManifestName(install); got != DefaultManifestName {
		t.Fatalf("ManifestName() = %q, want %q", got, DefaultManifestName)
	}
	if got := Mode(install); got != DefaultMode {
		t.Fatalf("Mode() = %q, want %q", got, DefaultMode)
	}
	if got := RenderedManifestSecretName(install); got != "edge-cilium-cilium-manifest" {
		t.Fatalf("RenderedManifestSecretName() = %q, want edge-cilium-cilium-manifest", got)
	}

	labels := RenderedManifestLabels(install)
	if labels[RenderedManifestOwnerLabel] != "edge-cilium" {
		t.Fatalf("owner label = %q, want edge-cilium", labels[RenderedManifestOwnerLabel])
	}
	if labels[RenderedManifestClusterLabel] != "edge" {
		t.Fatalf("cluster label = %q, want edge", labels[RenderedManifestClusterLabel])
	}

	install.Spec.ChartRepository = "https://charts.example.test/"
	install.Spec.ReleaseName = "network"
	install.Spec.Namespace = "networking"
	install.Spec.ManifestName = "network-cni"
	install.Spec.Mode = "one-time"

	if got := ChartRepository(install); got != "https://charts.example.test/" {
		t.Fatalf("ChartRepository() override = %q", got)
	}
	if got := ReleaseName(install); got != "network" {
		t.Fatalf("ReleaseName() override = %q", got)
	}
	if got := Namespace(install); got != "networking" {
		t.Fatalf("Namespace() override = %q", got)
	}
	if got := ManifestName(install); got != "network-cni" {
		t.Fatalf("ManifestName() override = %q", got)
	}
	if got := Mode(install); got != "one-time" {
		t.Fatalf("Mode() override = %q", got)
	}
}

func TestRenderedManifestHash(t *testing.T) {
	t.Parallel()

	manifest := []byte("apiVersion: v1\nkind: Namespace\n")
	sum := sha256.Sum256(manifest)
	want := hex.EncodeToString(sum[:])

	if got := RenderedManifestHash(manifest); got != want {
		t.Fatalf("RenderedManifestHash() = %q, want %q", got, want)
	}
}

func TestSpecHashUsesNormalizedInputs(t *testing.T) {
	t.Parallel()

	base := &omniv1alpha1.OmniCilium{
		Spec: omniv1alpha1.OmniCiliumSpec{
			ChartVersion: "1.18.3",
			Values: &apiextensionsv1.JSON{Raw: []byte(`{
				"kubeProxyReplacement": true,
				"securityContext": {"capabilities": {"ciliumAgent": ["` + netAdminCapability + `"]}}
			}`)},
		},
	}
	same := base.DeepCopy()
	same.Spec.ChartRepository = DefaultChartRepository
	same.Spec.ReleaseName = DefaultReleaseName
	same.Spec.Namespace = DefaultNamespace
	same.Spec.ManifestName = DefaultManifestName
	same.Spec.Mode = DefaultMode

	baseHash, err := SpecHash(base)
	if err != nil {
		t.Fatalf("SpecHash(base) error = %v", err)
	}
	sameHash, err := SpecHash(same)
	if err != nil {
		t.Fatalf("SpecHash(same) error = %v", err)
	}
	if baseHash != sameHash {
		t.Fatalf("hash with explicit defaults = %q, want %q", sameHash, baseHash)
	}

	changed := base.DeepCopy()
	changed.Spec.ManifestName = "other-cilium"
	changedHash, err := SpecHash(changed)
	if err != nil {
		t.Fatalf("SpecHash(changed) error = %v", err)
	}
	if changedHash == baseHash {
		t.Fatal("SpecHash() did not change when manifestName changed")
	}
}

func TestValuesMergesTalosDefaultsWithOverrides(t *testing.T) {
	t.Parallel()

	install := &omniv1alpha1.OmniCilium{
		Spec: omniv1alpha1.OmniCiliumSpec{
			Values: &apiextensionsv1.JSON{Raw: []byte(`{
				"ipam": {"operator": {"clusterPoolIPv4PodCIDRList": ["10.244.0.0/16"]}},
				"k8sServiceHost": "10.0.0.10",
				"kubeProxyReplacement": true,
				"securityContext": {"capabilities": {"ciliumAgent": ["` + netAdminCapability + `"]}}
			}`)},
		},
	}

	values, enabled, err := Values(install)
	if err != nil {
		t.Fatalf("Values() error = %v", err)
	}
	if !enabled {
		t.Fatal("kubeProxyReplacement enabled = false, want true")
	}
	if got := values["k8sServiceHost"]; got != "10.0.0.10" {
		t.Fatalf("k8sServiceHost = %#v, want caller override", got)
	}
	if got := values["k8sServicePort"]; got != 7445 {
		t.Fatalf("k8sServicePort = %#v, want default 7445", got)
	}

	ipam, ok := values["ipam"].(map[string]any)
	if !ok {
		t.Fatalf("ipam = %#v, want map", values["ipam"])
	}
	if got := ipam["mode"]; got != "kubernetes" {
		t.Fatalf("ipam.mode = %#v, want preserved Talos default", got)
	}
	operator, ok := ipam["operator"].(map[string]any)
	if !ok {
		t.Fatalf("ipam.operator = %#v, want merged override map", ipam["operator"])
	}
	if got := operator["clusterPoolIPv4PodCIDRList"]; got == nil {
		t.Fatal("ipam.operator.clusterPoolIPv4PodCIDRList missing")
	}

	securityContext, ok := values["securityContext"].(map[string]any)
	if !ok {
		t.Fatalf("securityContext = %#v, want map", values["securityContext"])
	}
	capabilities, ok := securityContext["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("securityContext.capabilities = %#v, want map", securityContext["capabilities"])
	}
	if got, ok := capabilities["cleanCiliumState"].([]any); !ok || len(got) == 0 {
		t.Fatalf("cleanCiliumState capabilities = %#v, want preserved default list", capabilities["cleanCiliumState"])
	}
	if got, ok := capabilities["ciliumAgent"].([]any); !ok || len(got) != 1 || got[0] != netAdminCapability {
		t.Fatalf("ciliumAgent capabilities = %#v, want caller replacement", capabilities["ciliumAgent"])
	}
}

func TestValuesAcceptsStringKubeProxyReplacement(t *testing.T) {
	t.Parallel()

	install := &omniv1alpha1.OmniCilium{
		Spec: omniv1alpha1.OmniCiliumSpec{
			Values: &apiextensionsv1.JSON{Raw: []byte(`{"kubeProxyReplacement":"strict"}`)},
		},
	}

	values, enabled, err := Values(install)
	if err != nil {
		t.Fatalf("Values() error = %v", err)
	}
	if !enabled {
		t.Fatal("kubeProxyReplacement enabled = false, want true")
	}
	if got := values["kubeProxyReplacement"]; got != "strict" {
		t.Fatalf("kubeProxyReplacement value = %#v, want original string", got)
	}
	if values["k8sServiceHost"] != "localhost" {
		t.Fatalf("k8sServiceHost = %#v, want localhost", values["k8sServiceHost"])
	}
}

func TestValuesAcceptsDisabledStringKubeProxyReplacement(t *testing.T) {
	t.Parallel()

	install := &omniv1alpha1.OmniCilium{
		Spec: omniv1alpha1.OmniCiliumSpec{
			Values: &apiextensionsv1.JSON{Raw: []byte(`{"kubeProxyReplacement":"disabled"}`)},
		},
	}

	values, enabled, err := Values(install)
	if err != nil {
		t.Fatalf("Values() error = %v", err)
	}
	if enabled {
		t.Fatal("kubeProxyReplacement enabled = true, want false")
	}
	if got := values["kubeProxyReplacement"]; got != "disabled" {
		t.Fatalf("kubeProxyReplacement value = %#v, want original string", got)
	}
	if _, ok := values["k8sServiceHost"]; ok {
		t.Fatalf("k8sServiceHost = %#v, want unset", values["k8sServiceHost"])
	}
}

func TestValuesAcceptsBooleanKubeProxyReplacement(t *testing.T) {
	t.Parallel()

	install := &omniv1alpha1.OmniCilium{
		Spec: omniv1alpha1.OmniCiliumSpec{
			Values: &apiextensionsv1.JSON{Raw: []byte(`{"kubeProxyReplacement":true}`)},
		},
	}

	values, enabled, err := Values(install)
	if err != nil {
		t.Fatalf("Values() error = %v", err)
	}
	if !enabled {
		t.Fatal("kubeProxyReplacement enabled = false, want true")
	}
	if got := values["k8sServicePort"]; got != 7445 {
		t.Fatalf("k8sServicePort = %#v, want 7445", got)
	}
}

func TestValuesRejectsUnknownStringKubeProxyReplacement(t *testing.T) {
	t.Parallel()

	install := &omniv1alpha1.OmniCilium{
		Spec: omniv1alpha1.OmniCiliumSpec{
			Values: &apiextensionsv1.JSON{Raw: []byte(`{"kubeProxyReplacement":"sometimes"}`)},
		},
	}

	_, _, err := Values(install)
	if err == nil {
		t.Fatal("Values() error = nil, want unsupported string error")
	}
	if !strings.Contains(err.Error(), `kubeProxyReplacement has unsupported string value "sometimes"`) {
		t.Fatalf("Values() error = %v, want unsupported string message", err)
	}
}

func TestValuesRejectsMalformedAndNonObjectValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		substring string
	}{
		{
			name:      "malformed json",
			raw:       `{"kubeProxyReplacement":`,
			substring: "decode cilium values",
		},
		{
			name:      "array",
			raw:       `[]`,
			substring: "cilium values must be a JSON object",
		},
		{
			name:      "wrong kubeProxyReplacement type",
			raw:       `{"kubeProxyReplacement":1}`,
			substring: "kubeProxyReplacement must be a boolean or recognized string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			install := &omniv1alpha1.OmniCilium{
				Spec: omniv1alpha1.OmniCiliumSpec{
					Values: &apiextensionsv1.JSON{Raw: []byte(tt.raw)},
				},
			}

			_, _, err := Values(install)
			if err == nil {
				t.Fatalf("Values() error = nil, want %q", tt.substring)
			}
			if !strings.Contains(err.Error(), tt.substring) {
				t.Fatalf("Values() error = %v, want substring %q", err, tt.substring)
			}
		})
	}
}

func TestParseRenderedManifestCompactsObjectsAndSkipsEmptyDocuments(t *testing.T) {
	t.Parallel()

	objects, err := ParseRenderedManifest([]byte(`
---
apiVersion: v1
kind: Namespace
metadata:
  name: kube-system
---
null
---
{}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cilium-config
  namespace: kube-system
data:
  mode: strict
`))
	if err != nil {
		t.Fatalf("ParseRenderedManifest() error = %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("objects length = %d, want 2: %#v", len(objects), objects)
	}

	var first map[string]any
	if err := json.Unmarshal(objects[0].Raw, &first); err != nil {
		t.Fatalf("unmarshal first object: %v", err)
	}
	if first["kind"] != "Namespace" {
		t.Fatalf("first kind = %#v, want Namespace", first["kind"])
	}
	if strings.Contains(string(objects[0].Raw), "\n") {
		t.Fatalf("first object was not compacted: %q", objects[0].Raw)
	}

	var second map[string]any
	if err := json.Unmarshal(objects[1].Raw, &second); err != nil {
		t.Fatalf("unmarshal second object: %v", err)
	}
	if second["kind"] != "ConfigMap" {
		t.Fatalf("second kind = %#v, want ConfigMap", second["kind"])
	}
}

func TestParseRenderedManifestRejectsEmptyAndInvalidDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		manifest  string
		substring string
	}{
		{
			name:      "empty",
			manifest:  "---\n{}\n---\nnull\n",
			substring: "rendered manifest contains no Kubernetes objects",
		},
		{
			name:      "invalid yaml",
			manifest:  "apiVersion: v1\nmetadata: [",
			substring: "convert manifest document to JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseRenderedManifest([]byte(tt.manifest))
			if err == nil {
				t.Fatalf("ParseRenderedManifest() error = nil, want %q", tt.substring)
			}
			if !strings.Contains(err.Error(), tt.substring) {
				t.Fatalf("ParseRenderedManifest() error = %v, want substring %q", err, tt.substring)
			}
		})
	}
}

func TestSecretHasCurrentManifest(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				RenderedManifestSpecHashKey: "current",
			},
		},
	}

	if SecretHasCurrentManifest(secret, nil, "current") {
		t.Fatal("SecretHasCurrentManifest() = true without manifest data, want false")
	}
	if SecretHasCurrentManifest(secret, map[string][]byte{RenderedManifestSecretKey: []byte("manifest")}, "stale") {
		t.Fatal("SecretHasCurrentManifest() = true for stale spec hash, want false")
	}
	if !SecretHasCurrentManifest(secret, map[string][]byte{RenderedManifestSecretKey: []byte("manifest")}, "current") {
		t.Fatal("SecretHasCurrentManifest() = false for current secret, want true")
	}
}

func TestHelmRendererSettingsUsesConfiguredCacheDir(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	settings, err := (HelmRenderer{CacheDir: cacheDir}).settings()
	if err != nil {
		t.Fatalf("settings() error = %v", err)
	}
	if settings.RepositoryCache != cacheDir {
		t.Fatalf("RepositoryCache = %q, want %q", settings.RepositoryCache, cacheDir)
	}
	if !strings.HasPrefix(settings.RepositoryConfig, cacheDir) {
		t.Fatalf("RepositoryConfig = %q, want under %q", settings.RepositoryConfig, cacheDir)
	}
	if !strings.HasPrefix(settings.RegistryConfig, cacheDir) {
		t.Fatalf("RegistryConfig = %q, want under %q", settings.RegistryConfig, cacheDir)
	}
}
