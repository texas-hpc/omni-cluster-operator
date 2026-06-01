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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/addon"
)

// AddonRenderer renders Helm manifests for an OmniClusterAddon resource.
type AddonRenderer interface {
	Render(context.Context, *omniv1alpha1.OmniClusterAddon) ([]byte, error)
}

// OmniClusterAddonReconciler reconciles an OmniClusterAddon object.
type OmniClusterAddonReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Renderer AddonRenderer
}

// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusteraddons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusteraddons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusteraddons/finalizers,verbs=update
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *OmniClusterAddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	item := &omniv1alpha1.OmniClusterAddon{}
	if err := r.Get(ctx, req.NamespacedName, item); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	exists, err := (childStatusClient{Client: r.Client, Scheme: r.Scheme}).clusterExists(ctx, item.Namespace, item.Spec.ClusterRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !exists {
		return ctrl.Result{}, updateAddonStatus(ctx, r.Client, item, false, false, "", nil)
	}

	specHash, err := addon.SpecHash(item)
	if err != nil {
		statusErr := updateAddonStatus(ctx, r.Client, item, true, false, "", err)
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{}, nil
	}

	secret := &corev1.Secret{}
	secretName := addon.RenderedManifestSecretName(item)
	secretKey := client.ObjectKey{Namespace: item.Namespace, Name: secretName}
	err = r.Get(ctx, secretKey, secret)
	if err == nil && addon.SecretHasCurrentManifest(secret, secret.Data, specHash) {
		hash := addon.RenderedManifestHash(secret.Data[addon.RenderedManifestSecretKey])
		return ctrl.Result{}, updateAddonStatus(ctx, r.Client, item, true, false, hash, nil)
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	manifest, renderErr := r.renderer().Render(ctx, item)
	if renderErr != nil {
		statusErr := updateAddonStatus(ctx, r.Client, item, true, false, "", renderErr)
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{}, nil
	}
	if _, parseErr := addon.ParseRenderedManifest(manifest); parseErr != nil {
		renderErr = fmt.Errorf("rendered manifest is invalid: %w", parseErr)
		statusErr := updateAddonStatus(ctx, r.Client, item, true, false, "", renderErr)
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{}, nil
	}

	if apierrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: item.Namespace,
				Name:      secretName,
			},
			Type: corev1.SecretTypeOpaque,
		}
	}

	hash := addon.RenderedManifestHash(manifest)
	secret.Labels = mergeStringMaps(secret.Labels, addon.RenderedManifestLabels(item))
	secret.Annotations = mergeStringMaps(secret.Annotations, map[string]string{
		addon.RenderedManifestSpecHashKey: specHash,
		addon.RenderedManifestHashKey:     hash,
	})
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data[addon.RenderedManifestSecretKey] = manifest
	if setErr := controllerutil.SetControllerReference(item, secret, r.Scheme); setErr != nil {
		return ctrl.Result{}, setErr
	}

	if apierrors.IsNotFound(err) {
		if createErr := r.Create(ctx, secret); createErr != nil {
			return ctrl.Result{}, createErr
		}
	} else if updateErr := r.Update(ctx, secret); updateErr != nil {
		return ctrl.Result{}, updateErr
	}

	log.V(1).Info("cached rendered addon manifest", "secret", secretKey.String(), "hash", hash)

	return ctrl.Result{}, updateAddonStatus(ctx, r.Client, item, true, true, hash, nil)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniClusterAddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniClusterAddon{}, builder.WithPredicates(specOrDeletionChangedPredicate())).
		Owns(&corev1.Secret{}).
		Watches(&omniv1alpha1.OmniCluster{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return addonRequestsForCluster(ctx, r.Client, object)
		}), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Named("omniclusteraddon").
		Complete(r)
}

func (r *OmniClusterAddonReconciler) renderer() AddonRenderer {
	if r.Renderer != nil {
		return r.Renderer
	}

	return addon.HelmRenderer{}
}
