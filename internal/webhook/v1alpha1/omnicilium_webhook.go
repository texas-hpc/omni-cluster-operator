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

// SetupOmniCiliumWebhookWithManager registers the webhook for OmniCilium in the manager.
func SetupOmniCiliumWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniv1alpha1.OmniCilium{}).
		WithValidator(&OmniCiliumCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-omni-texas-hpc-org-v1alpha1-omnicilium,mutating=false,failurePolicy=fail,sideEffects=None,groups=omni.texas-hpc.org,resources=omniciliums,verbs=create;update,versions=v1alpha1,name=vomnicilium-v1alpha1.kb.io,admissionReviewVersions=v1

// OmniCiliumCustomValidator struct is responsible for validating the OmniCilium resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for admission validation.
type OmniCiliumCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OmniCilium.
func (v *OmniCiliumCustomValidator) ValidateCreate(_ context.Context, obj *omniv1alpha1.OmniCilium) (admission.Warnings, error) {
	return nil, invalid("OmniCilium", obj.Name, validateCilium(obj))
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OmniCilium.
func (v *OmniCiliumCustomValidator) ValidateUpdate(_ context.Context, _ *omniv1alpha1.OmniCilium, newObj *omniv1alpha1.OmniCilium) (admission.Warnings, error) {
	return nil, invalid("OmniCilium", newObj.Name, validateCilium(newObj))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OmniCilium.
func (v *OmniCiliumCustomValidator) ValidateDelete(_ context.Context, _ *omniv1alpha1.OmniCilium) (admission.Warnings, error) {
	return nil, nil
}
