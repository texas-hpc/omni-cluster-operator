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

package secretsync

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const (
	testClusterName      = "edge"
	testSecretSyncName   = "edge-ghcr"
	testSourceSecretName = "source-ghcr"
	testTargetNamespace  = "flux-system"
	testUserValue        = "user-value"
)

func TestDesiredSecretCopiesSourceDataAndSetsManagedMetadata(t *testing.T) {
	t.Parallel()

	item := testSecretSync()
	item.Spec.Type = corev1.SecretTypeDockerConfigJson
	item.Spec.Labels = map[string]string{
		"team":         "platform",
		ManagedByLabel: testUserValue,
		OwnerUIDLabel:  testUserValue,
	}
	item.Spec.Annotations = map[string]string{
		"purpose":       "bootstrap",
		OwnerAnnotation: testUserValue,
	}
	source := testSourceSecret()

	desired := DesiredSecret(item, source, testClusterName)

	if desired.Name != item.Spec.TargetSecretRef.Name || desired.Namespace != item.Spec.TargetSecretRef.Namespace {
		t.Fatalf("target = %s/%s, want %s/%s", desired.Namespace, desired.Name, item.Spec.TargetSecretRef.Namespace, item.Spec.TargetSecretRef.Name)
	}
	if desired.Type != corev1.SecretTypeDockerConfigJson {
		t.Fatalf("Type = %q, want %q", desired.Type, corev1.SecretTypeDockerConfigJson)
	}
	if string(desired.Data[".dockerconfigjson"]) != `{"auths":{"ghcr.io":{}}}` {
		t.Fatalf("Data = %#v, want copied docker config", desired.Data)
	}
	if desired.Labels["team"] != "platform" {
		t.Fatalf("team label = %q, want platform", desired.Labels["team"])
	}
	if desired.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("managed-by label = %q, want %q", desired.Labels[ManagedByLabel], ManagedByValue)
	}
	if desired.Labels[OwnerUIDLabel] != string(item.UID) {
		t.Fatalf("owner UID label = %q, want %q", desired.Labels[OwnerUIDLabel], item.UID)
	}
	if desired.Labels[ClusterLabel] != testClusterName {
		t.Fatalf("cluster label = %q, want %q", desired.Labels[ClusterLabel], testClusterName)
	}
	if desired.Annotations["purpose"] != "bootstrap" {
		t.Fatalf("purpose annotation = %q, want bootstrap", desired.Annotations["purpose"])
	}
	if desired.Annotations[OwnerAnnotation] != item.Name {
		t.Fatalf("owner annotation = %q, want %q", desired.Annotations[OwnerAnnotation], item.Name)
	}
	if desired.Annotations[SourceAnnotation] != "default/source-ghcr" {
		t.Fatalf("source annotation = %q, want default/source-ghcr", desired.Annotations[SourceAnnotation])
	}

	source.Data[".dockerconfigjson"][0] = 'X'
	if string(desired.Data[".dockerconfigjson"]) != `{"auths":{"ghcr.io":{}}}` {
		t.Fatalf("desired data changed after source mutation: %q", desired.Data[".dockerconfigjson"])
	}
}

func TestHashIsStableAndIncludesSecretType(t *testing.T) {
	t.Parallel()

	first, err := Hash(corev1.SecretTypeOpaque, map[string][]byte{
		"b": []byte("two"),
		"a": []byte("one"),
	})
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	second, err := Hash(corev1.SecretTypeOpaque, map[string][]byte{
		"a": []byte("one"),
		"b": []byte("two"),
	})
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if first != second {
		t.Fatalf("hash order changed: %q != %q", first, second)
	}

	typed, err := Hash(corev1.SecretTypeDockerConfigJson, map[string][]byte{
		"a": []byte("one"),
		"b": []byte("two"),
	})
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if first == typed {
		t.Fatalf("hash did not change when Secret type changed: %q", first)
	}
}

func TestIsOwnedSecretUsesUIDThenNameAnnotation(t *testing.T) {
	t.Parallel()

	item := testSecretSync()
	owned := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				OwnerUIDLabel: string(item.UID),
			},
			Annotations: map[string]string{
				OwnerAnnotation: "other",
			},
		},
	}
	if !IsOwnedSecret(item, owned) {
		t.Fatal("IsOwnedSecret() = false, want true for matching UID label")
	}

	withoutUID := testSecretSync()
	withoutUID.UID = ""
	annotationOwned := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				OwnerAnnotation: withoutUID.Name,
			},
		},
	}
	if !IsOwnedSecret(withoutUID, annotationOwned) {
		t.Fatal("IsOwnedSecret() = false, want true for fallback owner annotation")
	}

	notOwned := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				OwnerUIDLabel: "other",
			},
			Annotations: map[string]string{
				OwnerAnnotation: item.Name,
			},
		},
	}
	if IsOwnedSecret(item, notOwned) {
		t.Fatal("IsOwnedSecret() = true, want false when UID label does not match")
	}
}

func TestCopyByteMapDeepCopiesValues(t *testing.T) {
	t.Parallel()

	input := map[string][]byte{"key": []byte("value")}
	output := copyByteMap(input)
	input["key"][0] = 'X'

	if !reflect.DeepEqual(output, map[string][]byte{"key": []byte("value")}) {
		t.Fatalf("copyByteMap() = %#v, want deep copy", output)
	}
}

func testSecretSync() *omniv1alpha1.OmniSecretSync {
	return &omniv1alpha1.OmniSecretSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretSyncName,
			Namespace: "default",
			UID:       "22222222-2222-4222-8222-222222222222",
		},
		Spec: omniv1alpha1.OmniSecretSyncSpec{
			TargetSecretRef: omniv1alpha1.SecretSyncTargetSecretRef{
				Name:      testSecretSyncName,
				Namespace: testTargetNamespace,
			},
		},
	}
}

func testSourceSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSourceSecretName,
			Namespace: "default",
			Labels: map[string]string{
				"ignored": "true",
			},
			Annotations: map[string]string{
				"ignored": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"ghcr.io":{}}}`),
		},
	}
}
