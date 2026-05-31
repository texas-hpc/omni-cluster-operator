package cilium

import (
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

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
