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

// SetupOmniConnectionWebhookWithManager registers the webhook for OmniConnection in the manager.
func SetupOmniConnectionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniv1alpha1.OmniConnection{}).
		WithValidator(&OmniConnectionCustomValidator{}).
		Complete()
}

// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-omni-texashpc-com-v1alpha1-omniconnection,mutating=false,failurePolicy=fail,sideEffects=None,groups=omni.texashpc.com,resources=omniconnections,verbs=create;update,versions=v1alpha1,name=vomniconnection-v1alpha1.kb.io,admissionReviewVersions=v1

// OmniConnectionCustomValidator struct is responsible for validating the OmniConnection resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OmniConnectionCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OmniConnection.
func (v *OmniConnectionCustomValidator) ValidateCreate(_ context.Context, obj *omniv1alpha1.OmniConnection) (admission.Warnings, error) {
	return connectionWarnings(obj), invalid("OmniConnection", obj.Name, validateConnection(obj))
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OmniConnection.
func (v *OmniConnectionCustomValidator) ValidateUpdate(_ context.Context, _ *omniv1alpha1.OmniConnection, newObj *omniv1alpha1.OmniConnection) (admission.Warnings, error) {
	return connectionWarnings(newObj), invalid("OmniConnection", newObj.Name, validateConnection(newObj))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OmniConnection.
func (v *OmniConnectionCustomValidator) ValidateDelete(_ context.Context, _ *omniv1alpha1.OmniConnection) (admission.Warnings, error) {
	return nil, nil
}
