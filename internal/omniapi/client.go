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

package omniapi

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	cosiresource "github.com/cosi-project/runtime/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniclient "github.com/siderolabs/omni/client/pkg/client"
	"github.com/siderolabs/omni/client/pkg/client/management"
	"github.com/siderolabs/omni/client/pkg/omni/resources"
	omniresources "github.com/siderolabs/omni/client/pkg/omni/resources/omni"
	omnioperations "github.com/siderolabs/omni/client/pkg/template/operations"
	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/omnitemplate"
)

// Client executes Omni operations needed by the Kubernetes reconcilers.
type Client interface {
	Ping(ctx context.Context, connection *omniv1alpha1.OmniConnection) (string, error)
	SyncTemplate(ctx context.Context, connection *omniv1alpha1.OmniConnection, templateBytes []byte, templateRoot string, options SyncOptions) (string, error)
	DeleteCluster(ctx context.Context, connection *omniv1alpha1.OmniConnection, clusterName string, options SyncOptions) (string, error)
	StatusCluster(ctx context.Context, connection *omniv1alpha1.OmniConnection, clusterName string) (string, error)
	ServiceAccountKubeconfig(ctx context.Context, connection *omniv1alpha1.OmniConnection, clusterName string, ttl time.Duration, user string, groups []string) ([]byte, error)
}

// SyncOptions are remote Omni sync/delete options.
type SyncOptions struct {
	DestroyMachines bool
}

// RealClient uses Sidero's Omni Go client.
type RealClient struct {
	K8sClient client.Reader
}

// Ping verifies auth and basic COSI resource access.
func (c *RealClient) Ping(ctx context.Context, connection *omniv1alpha1.OmniConnection) (string, error) {
	omniClient, err := c.newOmniClient(ctx, connection)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = omniClient.Close()
	}()

	if _, err = omniClient.Omni().State().List(ctx, cosiresource.NewMetadata(resources.DefaultNamespace, omniresources.ClusterType, "", cosiresource.VersionUndefined)); err != nil {
		return "", fmt.Errorf("list Omni clusters: %w", err)
	}

	return fmt.Sprintf("connected to %s", omniClient.Endpoint()), nil
}

// SyncTemplate validates and syncs a rendered Omni cluster template.
func (c *RealClient) SyncTemplate(ctx context.Context, connection *omniv1alpha1.OmniConnection, templateBytes []byte, templateRoot string, options SyncOptions) (string, error) {
	omniClient, err := c.newOmniClient(ctx, connection)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = omniClient.Close()
	}()

	root, closeRoot, err := omnitemplate.OpenRoot(templateRoot)
	if err != nil {
		return "", err
	}
	defer closeRoot()

	var out bytes.Buffer
	err = omnioperations.SyncTemplate(ctx, bytes.NewReader(templateBytes), &out, omniClient.Omni().State(), omnioperations.SyncOptions{
		DestroyMachines: options.DestroyMachines,
	}, root)
	if err != nil {
		return out.String(), err
	}

	return out.String(), nil
}

// DeleteCluster removes template-managed resources for a cluster from Omni.
func (c *RealClient) DeleteCluster(ctx context.Context, connection *omniv1alpha1.OmniConnection, clusterName string, options SyncOptions) (string, error) {
	omniClient, err := c.newOmniClient(ctx, connection)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = omniClient.Close()
	}()

	var out bytes.Buffer
	err = omnioperations.DeleteCluster(ctx, clusterName, &out, omniClient.Omni().State(), omnioperations.SyncOptions{
		DestroyMachines: options.DestroyMachines,
	})
	if err != nil {
		return out.String(), err
	}

	return out.String(), nil
}

// StatusCluster reads Omni cluster status without waiting for health.
func (c *RealClient) StatusCluster(ctx context.Context, connection *omniv1alpha1.OmniConnection, clusterName string) (string, error) {
	omniClient, err := c.newOmniClient(ctx, connection)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = omniClient.Close()
	}()

	var out bytes.Buffer
	err = omnioperations.StatusCluster(ctx, clusterName, &out, omniClient.Omni().State(), omnioperations.StatusOptions{
		Quiet: true,
	})
	if err != nil {
		return out.String(), err
	}

	return out.String(), nil
}

// ServiceAccountKubeconfig requests a service-account kubeconfig from Omni management API.
func (c *RealClient) ServiceAccountKubeconfig(ctx context.Context, connection *omniv1alpha1.OmniConnection, clusterName string, ttl time.Duration, user string, groups []string) (kubeconfig []byte, retErr error) {
	omniClient, err := c.newOmniClient(ctx, connection)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := omniClient.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("close Omni client: %w", closeErr)
		}
	}()

	kubeconfig, err = omniClient.Management().WithCluster(clusterName).Kubeconfig(ctx, management.WithServiceAccount(ttl, user, groups...))
	if err != nil {
		return nil, fmt.Errorf("request service-account kubeconfig for cluster %q: %w", clusterName, err)
	}

	return kubeconfig, nil
}

func (c *RealClient) newOmniClient(ctx context.Context, connection *omniv1alpha1.OmniConnection) (*omniclient.Client, error) {
	serviceAccountKey, err := c.serviceAccountKey(ctx, connection)
	if err != nil {
		return nil, err
	}

	omniClient, err := omniclient.New(connection.Spec.Endpoint,
		omniclient.WithServiceAccount(serviceAccountKey),
		omniclient.WithInsecureSkipTLSVerify(connection.Spec.InsecureSkipTLSVerify),
	)
	if err != nil {
		return nil, fmt.Errorf("create Omni client: %w", err)
	}

	return omniClient, nil
}

func (c *RealClient) serviceAccountKey(ctx context.Context, connection *omniv1alpha1.OmniConnection) (string, error) {
	ref := connection.Spec.Auth.ServiceAccountSecretRef
	secret := &corev1.Secret{}
	if err := c.K8sClient.Get(ctx, types.NamespacedName{Namespace: connection.Namespace, Name: ref.Name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("%s %q: %w", omniv1alpha1.ReasonMissingSecret, ref.Name, err)
		}

		return "", err
	}

	value, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s missing key %q", connection.Namespace, ref.Name, ref.Key)
	}

	serviceAccountKey := strings.TrimSpace(string(value))
	if serviceAccountKey == "" {
		return "", fmt.Errorf("secret %s/%s key %q is empty", connection.Namespace, ref.Name, ref.Key)
	}

	return serviceAccountKey, nil
}
