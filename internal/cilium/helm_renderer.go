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
	"fmt"
	"os"
	"path/filepath"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	helmrelease "helm.sh/helm/v4/pkg/release"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
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

	settings, err := r.settings()
	if err != nil {
		return nil, false, err
	}

	cfg := action.NewConfiguration()
	client := action.NewInstall(cfg)
	client.RepoURL = ChartRepository(install)
	client.Version = install.Spec.ChartVersion
	client.ReleaseName = ReleaseName(install)
	client.Namespace = Namespace(install)
	client.DryRunStrategy = action.DryRunClient
	client.Replace = true
	client.IncludeCRDs = true

	chartPath, err := client.LocateChart(ChartName, settings)
	if err != nil {
		return nil, false, fmt.Errorf("locate Cilium chart %s from %s: %w", install.Spec.ChartVersion, ChartRepository(install), err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, false, fmt.Errorf("load Cilium chart %q: %w", chartPath, err)
	}

	release, err := client.RunWithContext(ctx, chart, values)
	if err != nil {
		return nil, false, fmt.Errorf("render Cilium chart: %w", err)
	}
	accessor, err := helmrelease.NewAccessor(release)
	if err != nil {
		return nil, false, fmt.Errorf("render Cilium chart: read release: %w", err)
	}
	manifest := accessor.Manifest()
	if manifest == "" {
		return nil, false, fmt.Errorf("render Cilium chart: rendered manifest is empty")
	}

	return []byte(manifest), kubeProxyReplacement, nil
}

func (r HelmRenderer) settings() (*cli.EnvSettings, error) {
	cacheDir := r.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "omni-cluster-operator", "helm")
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create Helm cache directory %q: %w", cacheDir, err)
	}

	settings := cli.New()
	settings.RepositoryCache = cacheDir
	settings.RepositoryConfig = filepath.Join(cacheDir, "repositories.yaml")
	settings.RegistryConfig = filepath.Join(cacheDir, "registry.json")

	return settings, nil
}
