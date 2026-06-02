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

// SetupOmniKubeconfigExportWebhookWithManager registers the webhook for OmniKubeconfigExport in the manager.
func SetupOmniKubeconfigExportWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniv1alpha1.OmniKubeconfigExport{}).
		WithValidator(&OmniKubeconfigExportCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-omni-texashpc-com-v1alpha1-omnikubeconfigexport,mutating=false,failurePolicy=fail,sideEffects=None,groups=omni.texashpc.com,resources=omnikubeconfigexports,verbs=create;update,versions=v1alpha1,name=vomnikubeconfigexport-v1alpha1.kb.io,admissionReviewVersions=v1

// OmniKubeconfigExportCustomValidator validates OmniKubeconfigExport resources.
type OmniKubeconfigExportCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OmniKubeconfigExport.
func (v *OmniKubeconfigExportCustomValidator) ValidateCreate(_ context.Context, obj *omniv1alpha1.OmniKubeconfigExport) (admission.Warnings, error) {
	return kubeconfigExportWarnings(obj), invalid("OmniKubeconfigExport", obj.Name, validateKubeconfigExport(obj))
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OmniKubeconfigExport.
func (v *OmniKubeconfigExportCustomValidator) ValidateUpdate(_ context.Context, _ *omniv1alpha1.OmniKubeconfigExport, newObj *omniv1alpha1.OmniKubeconfigExport) (admission.Warnings, error) {
	return kubeconfigExportWarnings(newObj), invalid("OmniKubeconfigExport", newObj.Name, validateKubeconfigExport(newObj))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OmniKubeconfigExport.
func (v *OmniKubeconfigExportCustomValidator) ValidateDelete(_ context.Context, _ *omniv1alpha1.OmniKubeconfigExport) (admission.Warnings, error) {
	return nil, nil
}
