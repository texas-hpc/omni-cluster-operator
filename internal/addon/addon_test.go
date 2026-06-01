package addon

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const testAddonName = "apps"

func TestNormalizedSpecDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	item := testAddon()
	item.Spec.ManifestName = ""
	item.Spec.Mode = ""
	item.Spec.Helm.ReleaseName = ""
	item.Spec.Helm.Namespace = ""

	if got := ReleaseName(item); got != testAddonName {
		t.Fatalf("ReleaseName() = %q, want metadata name", got)
	}
	if got := Namespace(item); got != DefaultNamespace {
		t.Fatalf("Namespace() = %q, want %q", got, DefaultNamespace)
	}
	if got := ManifestName(item); got != testAddonName {
		t.Fatalf("ManifestName() = %q, want metadata name", got)
	}
	if got := Mode(item); got != DefaultMode {
		t.Fatalf("Mode() = %q, want %q", got, DefaultMode)
	}
	if got := RenderedManifestSecretName(item); got != testAddonName+"-addon-manifest" {
		t.Fatalf("RenderedManifestSecretName() = %q, want apps-addon-manifest", got)
	}

	labels := RenderedManifestLabels(item)
	if labels[RenderedManifestOwnerLabel] != testAddonName {
		t.Fatalf("owner label = %q, want apps", labels[RenderedManifestOwnerLabel])
	}
	if labels[RenderedManifestClusterLabel] != "edge" {
		t.Fatalf("cluster label = %q, want edge", labels[RenderedManifestClusterLabel])
	}

	item.Spec.ManifestName = "gateway"
	item.Spec.Mode = "one-time"
	item.Spec.Helm.ReleaseName = "gateway-release"
	item.Spec.Helm.Namespace = "networking"

	if got := ReleaseName(item); got != "gateway-release" {
		t.Fatalf("ReleaseName() override = %q", got)
	}
	if got := Namespace(item); got != "networking" {
		t.Fatalf("Namespace() override = %q", got)
	}
	if got := ManifestName(item); got != "gateway" {
		t.Fatalf("ManifestName() override = %q", got)
	}
	if got := Mode(item); got != "one-time" {
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

	base := testAddon()
	base.Spec.ManifestName = ""
	base.Spec.Mode = ""
	base.Spec.Helm.ReleaseName = ""
	base.Spec.Helm.Namespace = ""
	same := base.DeepCopy()
	same.Spec.ManifestName = base.Name
	same.Spec.Mode = DefaultMode
	same.Spec.Helm.ReleaseName = base.Name
	same.Spec.Helm.Namespace = DefaultNamespace

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
	changed.Spec.Helm.Values = &apiextensionsv1.JSON{Raw: []byte(`{"replicaCount": 3}`)}
	changedHash, err := SpecHash(changed)
	if err != nil {
		t.Fatalf("SpecHash(changed) error = %v", err)
	}
	if changedHash == baseHash {
		t.Fatal("SpecHash() did not change when values changed")
	}
}

func TestValuesRequiresJSONObject(t *testing.T) {
	t.Parallel()

	item := testAddon()
	item.Spec.Helm.Values = &apiextensionsv1.JSON{Raw: []byte(`[]`)}

	if _, err := Values(item); err == nil {
		t.Fatal("Values() error = nil, want object validation error")
	}
}

func TestParseRenderedManifestCompactsObjectsAndSkipsEmptyDocuments(t *testing.T) {
	t.Parallel()

	objects, err := ParseRenderedManifest([]byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: apps
---
null
---
{}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
`))
	if err != nil {
		t.Fatalf("ParseRenderedManifest() error = %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("ParseRenderedManifest() returned %d objects, want 2", len(objects))
	}
	if string(objects[0].Raw) != `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"apps"}}` {
		t.Fatalf("object[0] = %s", objects[0].Raw)
	}
}

func TestParseRenderedManifestAllowsEmptyDocuments(t *testing.T) {
	t.Parallel()

	objects, err := ParseRenderedManifest(nil)
	if err != nil {
		t.Fatalf("ParseRenderedManifest() error = %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("ParseRenderedManifest() returned %d objects, want 0", len(objects))
	}
}

func TestSecretHasCurrentManifest(t *testing.T) {
	t.Parallel()

	manifest := []byte("manifest")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				RenderedManifestSpecHashKey: "current",
				RenderedManifestHashKey:     RenderedManifestHash(manifest),
			},
		},
	}

	if SecretHasCurrentManifest(secret, map[string][]byte{RenderedManifestSecretKey: manifest}, "stale") {
		t.Fatal("SecretHasCurrentManifest() = true for stale spec hash")
	}
	if SecretHasCurrentManifest(secret, map[string][]byte{RenderedManifestSecretKey: []byte("corrupted")}, "current") {
		t.Fatal("SecretHasCurrentManifest() = true for corrupted manifest")
	}
	if !SecretHasCurrentManifest(secret, map[string][]byte{RenderedManifestSecretKey: manifest}, "current") {
		t.Fatal("SecretHasCurrentManifest() = false for current manifest")
	}

	emptyManifestSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				RenderedManifestSpecHashKey: "current",
				RenderedManifestHashKey:     RenderedManifestHash(nil),
			},
		},
	}
	if !SecretHasCurrentManifest(emptyManifestSecret, map[string][]byte{RenderedManifestSecretKey: nil}, "current") {
		t.Fatal("SecretHasCurrentManifest() = false for current empty manifest")
	}
}

func testAddon() *omniv1alpha1.OmniClusterAddon {
	return &omniv1alpha1.OmniClusterAddon{
		ObjectMeta: metav1.ObjectMeta{Name: testAddonName, Namespace: "default"},
		Spec: omniv1alpha1.OmniClusterAddonSpec{
			ClusterRef:   omniv1alpha1.OmniClusterRef{Name: "edge"},
			ManifestName: testAddonName,
			Mode:         "full",
			Helm: omniv1alpha1.OmniClusterAddonHelmSpec{
				Repository:  "https://charts.example.test/",
				Chart:       testAddonName,
				Version:     "1.2.3",
				ReleaseName: testAddonName,
				Namespace:   testAddonName,
				Values:      &apiextensionsv1.JSON{Raw: []byte(`{"replicaCount": 2}`)},
			},
		},
	}
}
