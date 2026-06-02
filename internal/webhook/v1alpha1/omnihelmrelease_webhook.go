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

package v1alpha1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
)

// SetupOmniHelmReleaseWebhookWithManager registers the webhook for OmniHelmRelease in the manager.
func SetupOmniHelmReleaseWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniv1alpha1.OmniHelmRelease{}).
		WithValidator(&OmniHelmReleaseCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-omni-texashpc-com-v1alpha1-omnihelmrelease,mutating=false,failurePolicy=fail,sideEffects=None,groups=omni.texashpc.com,resources=omnihelmreleases,verbs=create;update,versions=v1alpha1,name=vomnihelmrelease-v1alpha1.kb.io,admissionReviewVersions=v1

// OmniHelmReleaseCustomValidator validates OmniHelmRelease resources.
type OmniHelmReleaseCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OmniHelmRelease.
func (v *OmniHelmReleaseCustomValidator) ValidateCreate(_ context.Context, obj *omniv1alpha1.OmniHelmRelease) (admission.Warnings, error) {
	return helmReleaseWarnings(obj), invalid("OmniHelmRelease", obj.Name, validateHelmRelease(obj))
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OmniHelmRelease.
func (v *OmniHelmReleaseCustomValidator) ValidateUpdate(_ context.Context, _ *omniv1alpha1.OmniHelmRelease, newObj *omniv1alpha1.OmniHelmRelease) (admission.Warnings, error) {
	return helmReleaseWarnings(newObj), invalid("OmniHelmRelease", newObj.Name, validateHelmRelease(newObj))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OmniHelmRelease.
func (v *OmniHelmReleaseCustomValidator) ValidateDelete(_ context.Context, _ *omniv1alpha1.OmniHelmRelease) (admission.Warnings, error) {
	return nil, nil
}
