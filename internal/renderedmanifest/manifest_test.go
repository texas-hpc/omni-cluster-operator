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

package renderedmanifest

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseAllowsEmptyWhenConfigured(t *testing.T) {
	t.Parallel()

	objects, err := Parse([]byte(`
---
null
---
{}
`), AllowEmpty)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("Parse() returned %d objects, want 0", len(objects))
	}

	if _, err := Parse(nil); err == nil || !strings.Contains(err.Error(), "rendered manifest contains no Kubernetes objects") {
		t.Fatalf("Parse() error = %v, want empty manifest error", err)
	}
}

func TestSecretHasCurrentManifestValidatesSpecAndManifestHashes(t *testing.T) {
	t.Parallel()

	const (
		specHashAnnotation = "omni.texashpc.com/test-spec-hash"
		currentSpecHash    = "current-spec"
	)

	manifest := []byte("manifest")
	currentSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				specHashAnnotation: currentSpecHash,
				HashAnnotation:     Hash(manifest),
			},
		},
	}

	tests := []struct {
		name     string
		secret   *corev1.Secret
		data     map[string][]byte
		specHash string
		want     bool
	}{
		{
			name:     "current",
			secret:   currentSecret,
			data:     map[string][]byte{SecretKey: manifest},
			specHash: currentSpecHash,
			want:     true,
		},
		{
			name: "current empty manifest",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						specHashAnnotation: currentSpecHash,
						HashAnnotation:     Hash(nil),
					},
				},
			},
			data:     map[string][]byte{SecretKey: nil},
			specHash: currentSpecHash,
			want:     true,
		},
		{
			name:     "missing manifest key",
			secret:   currentSecret,
			data:     nil,
			specHash: currentSpecHash,
		},
		{
			name:     "stale spec hash",
			secret:   currentSecret,
			data:     map[string][]byte{SecretKey: manifest},
			specHash: "stale-spec",
		},
		{
			name: "missing rendered manifest hash",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						specHashAnnotation: currentSpecHash,
					},
				},
			},
			data:     map[string][]byte{SecretKey: manifest},
			specHash: currentSpecHash,
		},
		{
			name: "stale rendered manifest hash",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						specHashAnnotation: currentSpecHash,
						HashAnnotation:     Hash([]byte("other manifest")),
					},
				},
			},
			data:     map[string][]byte{SecretKey: manifest},
			specHash: currentSpecHash,
		},
		{
			name:     "nil annotations",
			secret:   &corev1.Secret{},
			data:     map[string][]byte{SecretKey: manifest},
			specHash: currentSpecHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SecretHasCurrentManifest(tt.secret, tt.data, specHashAnnotation, tt.specHash); got != tt.want {
				t.Fatalf("SecretHasCurrentManifest() = %v, want %v", got, tt.want)
			}
		})
	}
}
