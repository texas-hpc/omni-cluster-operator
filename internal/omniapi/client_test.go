package omniapi

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

func TestServiceAccountKey(t *testing.T) {
	t.Parallel()

	validKey := base64.StdEncoding.EncodeToString([]byte(`{"name":"automation","pgp_key":"private-key"}`))

	tests := []struct {
		name        string
		secretValue []byte
		want        string
		wantErr     string
	}{
		{
			name:        "base64 key",
			secretValue: []byte(validKey),
			want:        validKey,
		},
		{
			name:        "environment block",
			secretValue: []byte("OMNI_ENDPOINT=https://omni.example.test\nOMNI_SERVICE_ACCOUNT_KEY=" + validKey),
			wantErr:     "contains Omni environment assignments",
		},
		{
			name:        "invalid base64",
			secretValue: []byte("not a service account key"),
			wantErr:     "must contain the base64-encoded Omni service account key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatalf("add corev1 to scheme: %v", err)
			}

			connection := &omniv1alpha1.OmniConnection{
				ObjectMeta: metav1.ObjectMeta{Name: "omni", Namespace: "default"},
				Spec: omniv1alpha1.OmniConnectionSpec{
					Auth: omniv1alpha1.OmniAuthSpec{
						ServiceAccountSecretRef: omniv1alpha1.SecretKeySelector{Name: "omni-service-account", Key: "serviceAccountKey"},
					},
				},
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "omni-service-account", Namespace: "default"},
				Data:       map[string][]byte{"serviceAccountKey": tt.secretValue},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

			got, err := (&RealClient{K8sClient: client}).serviceAccountKey(context.Background(), connection)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("serviceAccountKey() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("serviceAccountKey() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("serviceAccountKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
