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

package kubeconfigexport

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const testExportName = "edge-automation"

func TestTargetSecretKeyDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	item := testExport()
	if got := TargetSecretKey(item); got != DefaultSecretKey {
		t.Fatalf("TargetSecretKey() = %q, want %q", got, DefaultSecretKey)
	}

	item.Spec.TargetSecretRef.Key = "workload.kubeconfig"
	if got := TargetSecretKey(item); got != "workload.kubeconfig" {
		t.Fatalf("TargetSecretKey() = %q, want explicit key", got)
	}
}

func TestTargetSecretNamespaceDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	item := testExport()
	item.Namespace = "omni-clusters"
	if got := TargetSecretNamespace(item); got != "omni-clusters" {
		t.Fatalf("TargetSecretNamespace() = %q, want export namespace", got)
	}

	item.Spec.TargetSecretRef.Namespace = "skypilot"
	if got := TargetSecretNamespace(item); got != "skypilot" {
		t.Fatalf("TargetSecretNamespace() = %q, want explicit namespace", got)
	}
}

func TestSpecHashTracksGeneratedKubeconfigInputs(t *testing.T) {
	t.Parallel()

	base := testExport()
	baseHash := mustSpecHash(t, base, "edge")

	secretNameOnly := base.DeepCopy()
	secretNameOnly.Spec.TargetSecretRef.Name = "different-secret"
	secretNameOnly.Spec.RenewBefore = &metav1.Duration{Duration: time.Hour}
	if got := mustSpecHash(t, secretNameOnly, "edge"); got != baseHash {
		t.Fatalf("SpecHash() changed for non-content fields: got %q, want %q", got, baseHash)
	}

	keyChanged := base.DeepCopy()
	keyChanged.Spec.TargetSecretRef.Key = "other"
	if got := mustSpecHash(t, keyChanged, "edge"); got == baseHash {
		t.Fatal("SpecHash() did not change when target key changed")
	}

	contextNamespaceChanged := base.DeepCopy()
	contextNamespaceChanged.Spec.ContextNamespace = "sky"
	if got := mustSpecHash(t, contextNamespaceChanged, "edge"); got == baseHash {
		t.Fatal("SpecHash() did not change when context namespace changed")
	}

	userChanged := base.DeepCopy()
	userChanged.Spec.ServiceAccount.User = "other-user"
	if got := mustSpecHash(t, userChanged, "edge"); got == baseHash {
		t.Fatal("SpecHash() did not change when service account user changed")
	}

	groupChanged := base.DeepCopy()
	groupChanged.Spec.ServiceAccount.Groups = []string{"other-group"}
	if got := mustSpecHash(t, groupChanged, "edge"); got == baseHash {
		t.Fatal("SpecHash() did not change when service account groups changed")
	}

	ttlChanged := base.DeepCopy()
	ttlChanged.Spec.TTL = metav1.Duration{Duration: 12 * time.Hour}
	if got := mustSpecHash(t, ttlChanged, "edge"); got == baseHash {
		t.Fatal("SpecHash() did not change when ttl changed")
	}

	clusterChanged := base.DeepCopy()
	if got := mustSpecHash(t, clusterChanged, "other"); got == baseHash {
		t.Fatal("SpecHash() did not change when cluster name changed")
	}
}

func TestSecretOwnershipUsesUIDWhenPresent(t *testing.T) {
	t.Parallel()

	item := testExport()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				OwnerUIDLabel: string(item.UID),
			},
			Annotations: map[string]string{
				OwnerAnnotation: "different-export",
			},
		},
	}
	if !IsOwnedSecret(item, secret) {
		t.Fatal("IsOwnedSecret() = false, want true for matching UID label")
	}

	secret.Labels[OwnerUIDLabel] = "different-uid"
	secret.Annotations[OwnerAnnotation] = item.Name
	if IsOwnedSecret(item, secret) {
		t.Fatal("IsOwnedSecret() = true, want false when UID is present but label does not match")
	}

	item.UID = ""
	if !IsOwnedSecret(item, secret) {
		t.Fatal("IsOwnedSecret() = false, want true for name annotation fallback without UID")
	}
}

func TestRotationDueUsesRenewBefore(t *testing.T) {
	t.Parallel()

	expiration := metav1.NewTime(time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC))
	renewBefore := &metav1.Duration{Duration: 4 * time.Hour}

	if got := NextRotationTime(expiration, renewBefore); !got.Time.Equal(expiration.Add(-4 * time.Hour)) {
		t.Fatalf("NextRotationTime() = %s, want %s", got.Time, expiration.Add(-4*time.Hour))
	}
	if RotationDue(expiration.Add(-5*time.Hour), expiration, renewBefore) {
		t.Fatal("RotationDue() = true before renewBefore window")
	}
	if !RotationDue(expiration.Add(-4*time.Hour), expiration, renewBefore) {
		t.Fatal("RotationDue() = false at renewBefore boundary")
	}
	if !RotationDue(expiration.Time, expiration, nil) {
		t.Fatal("RotationDue() = false at expiration without renewBefore")
	}
}

func TestAnnotationTime(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				ExpirationAnnotation: "2026-06-03T12:00:00Z",
			},
		},
	}
	got, err := AnnotationTime(secret, ExpirationAnnotation)
	if err != nil {
		t.Fatalf("AnnotationTime() error = %v", err)
	}
	if got == nil || !got.Time.Equal(time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("AnnotationTime() = %#v, want parsed timestamp", got)
	}

	secret.Annotations[ExpirationAnnotation] = "not-a-time"
	if _, err := AnnotationTime(secret, ExpirationAnnotation); err == nil {
		t.Fatal("AnnotationTime() error = nil, want parse error")
	}
}

func testExport() *omniv1alpha1.OmniKubeconfigExport {
	return &omniv1alpha1.OmniKubeconfigExport{
		ObjectMeta: metav1.ObjectMeta{
			Name: testExportName,
			UID:  "11111111-1111-4111-8111-111111111111",
		},
		Spec: omniv1alpha1.OmniKubeconfigExportSpec{
			ClusterRef: omniv1alpha1.OmniClusterRef{Name: "edge"},
			TargetSecretRef: omniv1alpha1.KubeconfigTargetSecretRef{
				Name: testExportName,
			},
			ServiceAccount: omniv1alpha1.KubeconfigServiceAccountSpec{
				User:   testExportName,
				Groups: []string{"cluster-automation"},
			},
			TTL:            metav1.Duration{Duration: 24 * time.Hour},
			DeletionPolicy: omniv1alpha1.KubeconfigExportDeletionPolicyDelete,
		},
	}
}

func mustSpecHash(t *testing.T, item *omniv1alpha1.OmniKubeconfigExport, clusterName string) string {
	t.Helper()

	hash, err := SpecHash(item, clusterName)
	if err != nil {
		t.Fatalf("SpecHash() error = %v", err)
	}

	return hash
}
