package helmrelease

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"helm.sh/helm/v4/pkg/action"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const (
	testReleaseName      = "apps"
	testReleaseNamespace = "apps-system"
)

func TestNormalizedSpecDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	item := testRelease()
	item.Spec.ReleaseName = ""
	item.Spec.Namespace = ""
	item.Spec.KubeconfigSecretRef.Key = ""
	item.Spec.Timeout = nil
	item.Spec.DeletionPolicy = ""

	if got := ReleaseName(item); got != item.Name {
		t.Fatalf("ReleaseName() = %q, want metadata name", got)
	}
	if got := Namespace(item); got != DefaultNamespace {
		t.Fatalf("Namespace() = %q, want %q", got, DefaultNamespace)
	}
	if got := KubeconfigSecretKey(item); got != DefaultKubeconfigKey {
		t.Fatalf("KubeconfigSecretKey() = %q, want %q", got, DefaultKubeconfigKey)
	}
	if got := Timeout(item); got != DefaultTimeout {
		t.Fatalf("Timeout() = %s, want %s", got, DefaultTimeout)
	}
	if got := DeletionPolicy(item); got != DefaultDeletionPolicy {
		t.Fatalf("DeletionPolicy() = %q, want %q", got, DefaultDeletionPolicy)
	}

	item.Spec.ReleaseName = testReleaseName
	item.Spec.Namespace = testReleaseNamespace
	item.Spec.KubeconfigSecretRef.Key = "workload.kubeconfig"
	item.Spec.Timeout = &metav1.Duration{Duration: 10 * time.Minute}
	item.Spec.DeletionPolicy = omniv1alpha1.HelmReleaseDeletionPolicyOrphan

	if got := ReleaseName(item); got != testReleaseName {
		t.Fatalf("ReleaseName() override = %q", got)
	}
	if got := Namespace(item); got != testReleaseNamespace {
		t.Fatalf("Namespace() override = %q", got)
	}
	if got := KubeconfigSecretKey(item); got != "workload.kubeconfig" {
		t.Fatalf("KubeconfigSecretKey() override = %q", got)
	}
	if got := Timeout(item); got != 10*time.Minute {
		t.Fatalf("Timeout() override = %s", got)
	}
	if got := DeletionPolicy(item); got != omniv1alpha1.HelmReleaseDeletionPolicyOrphan {
		t.Fatalf("DeletionPolicy() override = %q", got)
	}
}

func TestValuesRequiresJSONObject(t *testing.T) {
	t.Parallel()

	item := testRelease()
	item.Spec.Chart.Values = &apiextensionsv1.JSON{Raw: []byte(`[]`)}

	if _, err := Values(item); err == nil {
		t.Fatal("Values() error = nil, want object validation error")
	}
}

func TestChartLocatorUsesRepositoryForNamedChart(t *testing.T) {
	t.Parallel()

	item := testRelease()

	chart, repository := ChartLocator(item)
	if chart != testReleaseName {
		t.Fatalf("ChartLocator() chart = %q, want %q", chart, testReleaseName)
	}
	if repository != "https://charts.example.test/" {
		t.Fatalf("ChartLocator() repository = %q", repository)
	}
}

func TestChartLocatorUsesFullOCIReference(t *testing.T) {
	t.Parallel()

	item := testRelease()
	item.Spec.Chart.Chart = "oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator"
	item.Spec.Chart.Repository = "oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator"

	chart, repository := ChartLocator(item)
	if chart != item.Spec.Chart.Chart {
		t.Fatalf("ChartLocator() chart = %q, want OCI reference", chart)
	}
	if repository != "" {
		t.Fatalf("ChartLocator() repository = %q, want empty repository for OCI reference", repository)
	}
}

func TestActionConfigInitializesRegistryClient(t *testing.T) {
	t.Parallel()

	cfg, _, err := Client{CacheDir: t.TempDir()}.actionConfig(testRelease(), testKubeconfig())
	if err != nil {
		t.Fatalf("actionConfig() error = %v", err)
	}
	if cfg.RegistryClient == nil {
		t.Fatal("actionConfig() RegistryClient = nil, want OCI registry client")
	}
	if got := action.NewInstall(cfg).GetRegistryClient(); got != cfg.RegistryClient {
		t.Fatal("action.NewInstall() did not inherit registry client")
	}
}

func TestWithIsolatedWorkingDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}

	parent := t.TempDir()
	var isolated string
	err = withIsolatedWorkingDirectory(parent, func() error {
		var getErr error
		isolated, getErr = os.Getwd()

		return getErr
	})
	if err != nil {
		t.Fatalf("withIsolatedWorkingDirectory() error = %v", err)
	}

	after, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() after callback error = %v", err)
	}
	if after != original {
		t.Fatalf("working directory after callback = %q, want %q", after, original)
	}

	rel, err := filepath.Rel(parent, isolated)
	if err != nil {
		t.Fatalf("filepath.Rel() error = %v", err)
	}
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Fatalf("isolated working directory = %q, want child of %q", isolated, parent)
	}
}

func TestWithIsolatedWorkingDirectoryRestoresOnError(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}

	sentinelErr := errors.New("sentinel")
	if err := withIsolatedWorkingDirectory(t.TempDir(), func() error {
		return sentinelErr
	}); !errors.Is(err, sentinelErr) {
		t.Fatalf("withIsolatedWorkingDirectory() error = %v, want wrapped sentinel", err)
	}

	after, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() after error path = %v", err)
	}
	if after != original {
		t.Fatalf("working directory after error path = %q, want %q", after, original)
	}
}

func testRelease() *omniv1alpha1.OmniHelmRelease {
	return &omniv1alpha1.OmniHelmRelease{
		ObjectMeta: metav1.ObjectMeta{Name: testReleaseName, Namespace: "default"},
		Spec: omniv1alpha1.OmniHelmReleaseSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: "edge"},
			KubeconfigSecretRef: omniv1alpha1.HelmReleaseKubeconfigSecretRef{
				Name: "edge-kubeconfig",
			},
			ReleaseName: testReleaseName,
			Namespace:   testReleaseNamespace,
			Chart: omniv1alpha1.OmniHelmChartSpec{
				Repository: "https://charts.example.test/",
				Chart:      testReleaseName,
				Version:    "1.2.3",
				Values:     &apiextensionsv1.JSON{Raw: []byte(`{"replicaCount":2}`)},
			},
		},
	}
}

func testKubeconfig() []byte {
	return []byte(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: test-token
`)
}
