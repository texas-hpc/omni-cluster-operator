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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const (
	DefaultSecretKey = "kubeconfig"

	ManagedByLabel         = "app.kubernetes.io/managed-by"
	OwnerUIDLabel          = "omni.texashpc.com/kubeconfig-export-uid"
	ClusterLabel           = "omni.texashpc.com/cluster"
	ManagedByValue         = "omni-cluster-operator"
	OwnerAnnotation        = "omni.texashpc.com/kubeconfig-export-name"
	HashAnnotation         = "omni.texashpc.com/kubeconfig-hash"
	SpecHashAnnotation     = "omni.texashpc.com/kubeconfig-export-spec-hash"
	ExpirationAnnotation   = "omni.texashpc.com/kubeconfig-expiration-time"
	LastRotationAnnotation = "omni.texashpc.com/kubeconfig-last-rotation-time"
)

const specHashVersion = "v1"

// TargetSecretKey returns the normalized Secret data key for kubeconfig data.
func TargetSecretKey(item *omniv1alpha1.OmniKubeconfigExport) string {
	if item.Spec.TargetSecretRef.Key != "" {
		return item.Spec.TargetSecretRef.Key
	}

	return DefaultSecretKey
}

// Hash returns a SHA-256 hash for kubeconfig bytes.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

// SpecHash returns a stable hash of fields that affect generated kubeconfig contents.
func SpecHash(item *omniv1alpha1.OmniKubeconfigExport, clusterName string) (string, error) {
	normalized := struct {
		Version              string   `json:"version"`
		ClusterName          string   `json:"clusterName"`
		TargetSecretKey      string   `json:"targetSecretKey"`
		ServiceAccountUser   string   `json:"serviceAccountUser"`
		ServiceAccountGroups []string `json:"serviceAccountGroups"`
		TTL                  string   `json:"ttl"`
	}{
		Version:              specHashVersion,
		ClusterName:          clusterName,
		TargetSecretKey:      TargetSecretKey(item),
		ServiceAccountUser:   item.Spec.ServiceAccount.User,
		ServiceAccountGroups: append([]string(nil), item.Spec.ServiceAccount.Groups...),
		TTL:                  item.Spec.TTL.Duration.String(),
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal kubeconfig export spec hash payload: %w", err)
	}

	return Hash(payload), nil
}

// SecretLabels returns labels used to identify a kubeconfig export target Secret.
func SecretLabels(item *omniv1alpha1.OmniKubeconfigExport, clusterName string) map[string]string {
	return map[string]string{
		ManagedByLabel: ManagedByValue,
		OwnerUIDLabel:  string(item.UID),
		ClusterLabel:   clusterName,
	}
}

// SecretAnnotations returns annotations that describe generated kubeconfig data.
func SecretAnnotations(item *omniv1alpha1.OmniKubeconfigExport, specHash, kubeconfigHash string, expirationTime, lastRotationTime metav1.Time) map[string]string {
	return map[string]string{
		OwnerAnnotation:        item.Name,
		SpecHashAnnotation:     specHash,
		HashAnnotation:         kubeconfigHash,
		ExpirationAnnotation:   expirationTime.UTC().Format(time.RFC3339),
		LastRotationAnnotation: lastRotationTime.UTC().Format(time.RFC3339),
	}
}

// IsOwnedSecret reports whether secret was written by item.
func IsOwnedSecret(item *omniv1alpha1.OmniKubeconfigExport, secret *corev1.Secret) bool {
	if secret == nil {
		return false
	}
	if item.UID != "" {
		return secret.Labels[OwnerUIDLabel] == string(item.UID)
	}

	return secret.Annotations[OwnerAnnotation] == item.Name
}

// NextRotationTime returns when rotation is next due for an expiration timestamp.
func NextRotationTime(expiration metav1.Time, renewBefore *metav1.Duration) metav1.Time {
	if renewBefore == nil || renewBefore.Duration <= 0 {
		return expiration
	}

	return metav1.NewTime(expiration.Add(-renewBefore.Duration))
}

// RotationDue reports whether existing kubeconfig data should be replaced.
func RotationDue(now time.Time, expiration metav1.Time, renewBefore *metav1.Duration) bool {
	nextRotation := NextRotationTime(expiration, renewBefore)

	return !now.Before(nextRotation.Time)
}

// AnnotationTime parses a timestamp annotation from a Secret.
func AnnotationTime(secret *corev1.Secret, key string) (*metav1.Time, error) {
	if secret == nil || secret.Annotations == nil || secret.Annotations[key] == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, secret.Annotations[key])
	if err != nil {
		return nil, err
	}

	value := metav1.NewTime(parsed)

	return &value, nil
}
