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
	"maps"

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
	"github.com/texas-hpc/omni-cluster-operator/internal/cilium"
)

// CiliumRenderer renders Cilium manifests for an OmniCilium resource.
type CiliumRenderer interface {
	Render(context.Context, *omniv1alpha1.OmniCilium) ([]byte, bool, error)
}

// OmniCiliumReconciler reconciles a OmniCilium object
type OmniCiliumReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Renderer CiliumRenderer
}

// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniciliums,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniciliums/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniciliums/finalizers,verbs=update
// +kubebuilder:rbac:groups=omni.texashpc.com,resources=omniclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *OmniCiliumReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	install := &omniv1alpha1.OmniCilium{}
	if err := r.Get(ctx, req.NamespacedName, install); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	exists, err := (childStatusClient{Client: r.Client, Scheme: r.Scheme}).clusterExists(ctx, install.Namespace, install.Spec.ClusterRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !exists {
		return ctrl.Result{}, updateCiliumStatus(ctx, r.Client, install, false, false, "", nil)
	}

	specHash, err := cilium.SpecHash(install)
	if err != nil {
		statusErr := updateCiliumStatus(ctx, r.Client, install, true, false, "", err)
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{}, nil
	}

	secret := &corev1.Secret{}
	secretName := cilium.RenderedManifestSecretName(install)
	secretKey := client.ObjectKey{Namespace: install.Namespace, Name: secretName}
	err = r.Get(ctx, secretKey, secret)
	if err == nil && cilium.SecretHasCurrentManifest(secret, secret.Data, specHash) {
		hash := cilium.RenderedManifestHash(secret.Data[cilium.RenderedManifestSecretKey])
		return ctrl.Result{}, updateCiliumStatus(ctx, r.Client, install, true, false, hash, nil)
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	manifest, _, renderErr := r.renderer().Render(ctx, install)
	if renderErr != nil {
		statusErr := updateCiliumStatus(ctx, r.Client, install, true, false, "", renderErr)
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{}, renderErr
	}
	if _, parseErr := cilium.ParseRenderedManifest(manifest); parseErr != nil {
		renderErr = fmt.Errorf("rendered manifest is invalid: %w", parseErr)
		statusErr := updateCiliumStatus(ctx, r.Client, install, true, false, "", renderErr)
		if statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{}, renderErr
	}

	if apierrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: install.Namespace,
				Name:      secretName,
			},
			Type: corev1.SecretTypeOpaque,
		}
	}

	hash := cilium.RenderedManifestHash(manifest)
	secret.Labels = mergeStringMaps(secret.Labels, cilium.RenderedManifestLabels(install))
	secret.Annotations = mergeStringMaps(secret.Annotations, map[string]string{
		cilium.RenderedManifestSpecHashKey: specHash,
		cilium.RenderedManifestHashKey:     hash,
	})
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data[cilium.RenderedManifestSecretKey] = manifest
	if setErr := controllerutil.SetControllerReference(install, secret, r.Scheme); setErr != nil {
		return ctrl.Result{}, setErr
	}

	if apierrors.IsNotFound(err) {
		if createErr := r.Create(ctx, secret); createErr != nil {
			return ctrl.Result{}, createErr
		}
	} else if updateErr := r.Update(ctx, secret); updateErr != nil {
		return ctrl.Result{}, updateErr
	}

	log.V(1).Info("cached rendered Cilium manifest", "secret", secretKey.String(), "hash", hash)

	return ctrl.Result{}, updateCiliumStatus(ctx, r.Client, install, true, true, hash, nil)
}

// SetupWithManager sets up the controller with the Manager.
func (r *OmniCiliumReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniv1alpha1.OmniCilium{}, builder.WithPredicates(specOrDeletionChangedPredicate())).
		Owns(&corev1.Secret{}).
		Watches(&omniv1alpha1.OmniCluster{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []ctrl.Request {
			return ciliumRequestsForCluster(ctx, r.Client, object)
		}), builder.WithPredicates(specOrDeletionChangedPredicate())).
		Named("omnicilium").
		Complete(r)
}

func (r *OmniCiliumReconciler) renderer() CiliumRenderer {
	if r.Renderer != nil {
		return r.Renderer
	}

	return cilium.HelmRenderer{}
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
