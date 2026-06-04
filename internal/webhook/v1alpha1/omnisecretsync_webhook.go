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

// SetupOmniSecretSyncWebhookWithManager registers the webhook for OmniSecretSync in the manager.
func SetupOmniSecretSyncWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniv1alpha1.OmniSecretSync{}).
		WithValidator(&OmniSecretSyncCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-omni-texashpc-com-v1alpha1-omnisecretsync,mutating=false,failurePolicy=fail,sideEffects=None,groups=omni.texashpc.com,resources=omnisecretsyncs,verbs=create;update,versions=v1alpha1,name=vomnisecretsync-v1alpha1.kb.io,admissionReviewVersions=v1

// OmniSecretSyncCustomValidator validates OmniSecretSync resources.
type OmniSecretSyncCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OmniSecretSync.
func (v *OmniSecretSyncCustomValidator) ValidateCreate(_ context.Context, obj *omniv1alpha1.OmniSecretSync) (admission.Warnings, error) {
	return secretSyncWarnings(obj), invalid("OmniSecretSync", obj.Name, validateSecretSync(obj))
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OmniSecretSync.
func (v *OmniSecretSyncCustomValidator) ValidateUpdate(_ context.Context, _ *omniv1alpha1.OmniSecretSync, newObj *omniv1alpha1.OmniSecretSync) (admission.Warnings, error) {
	return secretSyncWarnings(newObj), invalid("OmniSecretSync", newObj.Name, validateSecretSync(newObj))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OmniSecretSync.
func (v *OmniSecretSyncCustomValidator) ValidateDelete(_ context.Context, _ *omniv1alpha1.OmniSecretSync) (admission.Warnings, error) {
	return nil, nil
}
