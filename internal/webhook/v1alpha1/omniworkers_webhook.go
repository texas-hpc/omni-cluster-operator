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

// SetupOmniWorkersWebhookWithManager registers the webhook for OmniWorkers in the manager.
func SetupOmniWorkersWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniv1alpha1.OmniWorkers{}).
		WithValidator(&OmniWorkersCustomValidator{}).
		Complete()
}

// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-omni-texashpc-com-v1alpha1-omniworkers,mutating=false,failurePolicy=fail,sideEffects=None,groups=omni.texashpc.com,resources=omniworkers,verbs=create;update,versions=v1alpha1,name=vomniworkers-v1alpha1.kb.io,admissionReviewVersions=v1

// OmniWorkersCustomValidator struct is responsible for validating the OmniWorkers resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OmniWorkersCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OmniWorkers.
func (v *OmniWorkersCustomValidator) ValidateCreate(_ context.Context, obj *omniv1alpha1.OmniWorkers) (admission.Warnings, error) {
	return nil, invalid("OmniWorkers", obj.Name, validateWorkers(obj))
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OmniWorkers.
func (v *OmniWorkersCustomValidator) ValidateUpdate(_ context.Context, _ *omniv1alpha1.OmniWorkers, newObj *omniv1alpha1.OmniWorkers) (admission.Warnings, error) {
	return nil, invalid("OmniWorkers", newObj.Name, validateWorkers(newObj))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OmniWorkers.
func (v *OmniWorkersCustomValidator) ValidateDelete(_ context.Context, _ *omniv1alpha1.OmniWorkers) (admission.Warnings, error) {
	return nil, nil
}
