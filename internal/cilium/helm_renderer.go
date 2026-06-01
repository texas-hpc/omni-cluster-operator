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

package cilium

import (
	"context"

	"helm.sh/helm/v4/pkg/cli"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/helmchart"
)

// HelmRenderer renders the Cilium Helm chart without contacting the workload cluster.
type HelmRenderer struct {
	CacheDir string
}

// Render renders Cilium manifests for the installation spec.
func (r HelmRenderer) Render(ctx context.Context, install *omniv1alpha1.OmniCilium) ([]byte, bool, error) {
	values, kubeProxyReplacement, err := Values(install)
	if err != nil {
		return nil, false, err
	}

	manifest, err := helmchart.Renderer{CacheDir: r.CacheDir}.Render(ctx, helmchart.Spec{
		Repository:  ChartRepository(install),
		Chart:       ChartName,
		Version:     install.Spec.ChartVersion,
		ReleaseName: ReleaseName(install),
		Namespace:   Namespace(install),
		Values:      values,
	})
	if err != nil {
		return nil, false, err
	}

	return manifest, kubeProxyReplacement, nil
}

func (r HelmRenderer) settings() (*cli.EnvSettings, error) {
	return helmchart.Renderer{CacheDir: r.CacheDir}.Settings()
}
