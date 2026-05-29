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

// SetupOmniControlPlaneWebhookWithManager registers the webhook for OmniControlPlane in the manager.
func SetupOmniControlPlaneWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniv1alpha1.OmniControlPlane{}).
		WithValidator(&OmniControlPlaneCustomValidator{}).
		Complete()
}

// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-omni-texas-hpc-org-v1alpha1-omnicontrolplane,mutating=false,failurePolicy=fail,sideEffects=None,groups=omni.texas-hpc.org,resources=omnicontrolplanes,verbs=create;update,versions=v1alpha1,name=vomnicontrolplane-v1alpha1.kb.io,admissionReviewVersions=v1

// OmniControlPlaneCustomValidator struct is responsible for validating the OmniControlPlane resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OmniControlPlaneCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OmniControlPlane.
func (v *OmniControlPlaneCustomValidator) ValidateCreate(_ context.Context, obj *omniv1alpha1.OmniControlPlane) (admission.Warnings, error) {
	return nil, invalid("OmniControlPlane", obj.Name, validateControlPlane(obj))
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OmniControlPlane.
func (v *OmniControlPlaneCustomValidator) ValidateUpdate(_ context.Context, _ *omniv1alpha1.OmniControlPlane, newObj *omniv1alpha1.OmniControlPlane) (admission.Warnings, error) {
	return nil, invalid("OmniControlPlane", newObj.Name, validateControlPlane(newObj))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OmniControlPlane.
func (v *OmniControlPlaneCustomValidator) ValidateDelete(_ context.Context, _ *omniv1alpha1.OmniControlPlane) (admission.Warnings, error) {
	return nil, nil
}
