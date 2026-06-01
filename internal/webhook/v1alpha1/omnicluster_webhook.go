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

// SetupOmniClusterWebhookWithManager registers the webhook for OmniCluster in the manager.
func SetupOmniClusterWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniv1alpha1.OmniCluster{}).
		WithValidator(&OmniClusterCustomValidator{}).
		Complete()
}

// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-omni-texashpc-com-v1alpha1-omnicluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=omni.texashpc.com,resources=omniclusters,verbs=create;update,versions=v1alpha1,name=vomnicluster-v1alpha1.kb.io,admissionReviewVersions=v1

// OmniClusterCustomValidator struct is responsible for validating the OmniCluster resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type OmniClusterCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OmniCluster.
func (v *OmniClusterCustomValidator) ValidateCreate(_ context.Context, obj *omniv1alpha1.OmniCluster) (admission.Warnings, error) {
	return clusterWarnings(obj), invalid("OmniCluster", obj.Name, validateCluster(obj))
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OmniCluster.
func (v *OmniClusterCustomValidator) ValidateUpdate(_ context.Context, _ *omniv1alpha1.OmniCluster, newObj *omniv1alpha1.OmniCluster) (admission.Warnings, error) {
	return clusterWarnings(newObj), invalid("OmniCluster", newObj.Name, validateCluster(newObj))
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OmniCluster.
func (v *OmniClusterCustomValidator) ValidateDelete(_ context.Context, _ *omniv1alpha1.OmniCluster) (admission.Warnings, error) {
	return nil, nil
}
