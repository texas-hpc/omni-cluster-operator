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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

const (
	DefaultKubeconfigKey = "kubeconfig"

	ManagedByLabel    = "app.kubernetes.io/managed-by"
	OwnerUIDLabel     = "omni.texashpc.com/secret-sync-uid"
	ClusterLabel      = "omni.texashpc.com/cluster"
	ManagedByValue    = "omni-cluster-operator"
	OwnerAnnotation   = "omni.texashpc.com/secret-sync-name"
	HashAnnotation    = "omni.texashpc.com/secret-sync-hash"
	SourceAnnotation  = "omni.texashpc.com/secret-sync-source"
	ContentAnnotation = "omni.texashpc.com/secret-sync-content"
)

const contentHashVersion = "v1"

// Target identifies a workload-cluster Secret.
type Target struct {
	Namespace string
	Name      string
}

// Result describes a completed workload-cluster Secret sync.
type Result struct {
	Target Target
	Type   corev1.SecretType
	Hash   string
}

// Client syncs Secrets directly against a workload cluster.
type Client struct{}

// KubeconfigSecretKey returns the normalized Secret data key for workload kubeconfig data.
func KubeconfigSecretKey(item *omniv1alpha1.OmniSecretSync) string {
	if item.Spec.KubeconfigSecretRef.Key != "" {
		return item.Spec.KubeconfigSecretRef.Key
	}

	return DefaultKubeconfigKey
}

// TargetForItem returns the workload-cluster target configured by item.
func TargetForItem(item *omniv1alpha1.OmniSecretSync) Target {
	return Target{
		Namespace: item.Spec.TargetSecretRef.Namespace,
		Name:      item.Spec.TargetSecretRef.Name,
	}
}

// Sync creates or updates the workload-cluster Secret to match the source Secret.
func (c Client) Sync(ctx context.Context, item *omniv1alpha1.OmniSecretSync, source *corev1.Secret, kubeconfig []byte, clusterName string) (*Result, error) {
	clientset, err := c.clientset(kubeconfig)
	if err != nil {
		return nil, err
	}

	target := TargetForItem(item)
	if item.Spec.CreateNamespace {
		if err := ensureNamespace(ctx, clientset, target.Namespace); err != nil {
			return nil, err
		}
	}

	desired := DesiredSecret(item, source, clusterName)
	hash, err := Hash(desired.Type, desired.Data)
	if err != nil {
		return nil, err
	}
	desired.Annotations = mergeStringMaps(desired.Annotations, map[string]string{
		HashAnnotation:    hash,
		ContentAnnotation: contentHashVersion,
	})

	secrets := clientset.CoreV1().Secrets(target.Namespace)
	existing, err := secrets.Get(ctx, target.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, createErr := secrets.Create(ctx, desired, metav1.CreateOptions{}); createErr != nil {
			return nil, fmt.Errorf("create workload Secret %q: %w", namespacedName(target), createErr)
		}

		return &Result{Target: target, Type: desired.Type, Hash: hash}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get workload Secret %q: %w", namespacedName(target), err)
	}

	desired.ResourceVersion = existing.ResourceVersion
	if existing.Type == desired.Type &&
		reflect.DeepEqual(existing.Data, desired.Data) &&
		reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.Annotations, desired.Annotations) {
		return &Result{Target: target, Type: desired.Type, Hash: hash}, nil
	}

	if _, err := secrets.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return nil, fmt.Errorf("update workload Secret %q: %w", namespacedName(target), err)
	}

	return &Result{Target: target, Type: desired.Type, Hash: hash}, nil
}

// Delete deletes the workload-cluster Secret if it is owned by item.
func (c Client) Delete(ctx context.Context, item *omniv1alpha1.OmniSecretSync, kubeconfig []byte, target Target) error {
	clientset, err := c.clientset(kubeconfig)
	if err != nil {
		return err
	}

	secrets := clientset.CoreV1().Secrets(target.Namespace)
	secret, err := secrets.Get(ctx, target.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get workload Secret %q: %w", namespacedName(target), err)
	}
	if !IsOwnedSecret(item, secret) {
		return nil
	}

	if err := secrets.Delete(ctx, target.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete workload Secret %q: %w", namespacedName(target), err)
	}

	return nil
}

func (c Client) clientset(kubeconfig []byte) (kubernetes.Interface, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("load workload kubeconfig: %w", err)
	}

	config = rest.AddUserAgent(config, "omni-cluster-operator-secret-sync")
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create workload Kubernetes client: %w", err)
	}

	return clientset, nil
}

// DesiredSecret builds the target workload-cluster Secret.
func DesiredSecret(item *omniv1alpha1.OmniSecretSync, source *corev1.Secret, clusterName string) *corev1.Secret {
	target := TargetForItem(item)
	secretType := source.Type
	if item.Spec.Type != "" {
		secretType = item.Spec.Type
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        target.Name,
			Namespace:   target.Namespace,
			Labels:      mergeStringMaps(item.Spec.Labels, SecretLabels(item, clusterName)),
			Annotations: mergeStringMaps(item.Spec.Annotations, SecretAnnotations(item, source)),
		},
		Type: secretType,
		Data: copyByteMap(source.Data),
	}
}

// Hash returns a stable SHA-256 hash for synced Secret content.
func Hash(secretType corev1.SecretType, data map[string][]byte) (string, error) {
	payload := struct {
		Version string            `json:"version"`
		Type    corev1.SecretType `json:"type"`
		Data    map[string][]byte `json:"data"`
	}{
		Version: contentHashVersion,
		Type:    secretType,
		Data:    copyByteMap(data),
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal Secret hash payload: %w", err)
	}

	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:]), nil
}

// SecretLabels returns labels used to identify a workload-cluster Secret sync target.
func SecretLabels(item *omniv1alpha1.OmniSecretSync, clusterName string) map[string]string {
	return map[string]string{
		ManagedByLabel: ManagedByValue,
		OwnerUIDLabel:  string(item.UID),
		ClusterLabel:   clusterName,
	}
}

// SecretAnnotations returns annotations that identify the source of a synced Secret.
func SecretAnnotations(item *omniv1alpha1.OmniSecretSync, source *corev1.Secret) map[string]string {
	return map[string]string{
		OwnerAnnotation:  item.Name,
		SourceAnnotation: types.NamespacedName{Namespace: source.Namespace, Name: source.Name}.String(),
	}
}

// IsOwnedSecret reports whether secret was written by item.
func IsOwnedSecret(item *omniv1alpha1.OmniSecretSync, secret *corev1.Secret) bool {
	if secret == nil {
		return false
	}
	if item.UID != "" {
		return secret.Labels[OwnerUIDLabel] == string(item.UID)
	}

	return secret.Annotations[OwnerAnnotation] == item.Name
}

func ensureNamespace(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	namespaces := clientset.CoreV1().Namespaces()
	_, err := namespaces.Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get workload namespace %q: %w", namespace, err)
	}

	_, err = namespaces.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create workload namespace %q: %w", namespace, err)
	}

	return nil
}

func namespacedName(target Target) string {
	return types.NamespacedName{Namespace: target.Namespace, Name: target.Name}.String()
}

func copyByteMap(input map[string][]byte) map[string][]byte {
	if len(input) == 0 {
		return nil
	}

	output := make(map[string][]byte, len(input))
	for key, value := range input {
		output[key] = append([]byte(nil), value...)
	}

	return output
}

func mergeStringMaps(base, overrides map[string]string) map[string]string {
	if len(base) == 0 && len(overrides) == 0 {
		return nil
	}

	merged := make(map[string]string, len(base)+len(overrides))
	maps.Copy(merged, base)
	maps.Copy(merged, overrides)

	return merged
}
