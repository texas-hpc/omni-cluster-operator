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

package helmchart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	helmrelease "helm.sh/helm/v4/pkg/release"
)

// Spec is one Helm chart render request.
type Spec struct {
	Repository  string
	Chart       string
	Version     string
	ReleaseName string
	Namespace   string
	Values      map[string]any
}

// Renderer renders Helm charts without contacting the workload cluster.
type Renderer struct {
	CacheDir string
}

var locateChartMu sync.Mutex

// Render renders a chart to raw multi-document YAML.
func (r Renderer) Render(ctx context.Context, spec Spec) ([]byte, error) {
	settings, err := r.Settings()
	if err != nil {
		return nil, err
	}

	cfg := action.NewConfiguration()
	if err := cfg.Init(settings.RESTClientGetter(), spec.Namespace, "memory"); err != nil {
		return nil, fmt.Errorf("initialize Helm action configuration: %w", err)
	}

	client := action.NewInstall(cfg)
	client.RepoURL = spec.Repository
	client.Version = spec.Version
	client.ReleaseName = spec.ReleaseName
	client.Namespace = spec.Namespace
	client.DryRunStrategy = action.DryRunClient
	client.Replace = true
	client.IncludeCRDs = true

	locateChartMu.Lock()
	chartPath, err := client.LocateChart(spec.Chart, settings)
	locateChartMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("locate Helm chart %s %s from %s: %w", spec.Chart, spec.Version, spec.Repository, err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("load Helm chart %q: %w", chartPath, err)
	}

	release, err := client.RunWithContext(ctx, chart, spec.Values)
	if err != nil {
		return nil, fmt.Errorf("render Helm chart %s: %w", spec.Chart, err)
	}
	accessor, err := helmrelease.NewAccessor(release)
	if err != nil {
		return nil, fmt.Errorf("render Helm chart %s: read release: %w", spec.Chart, err)
	}
	manifest := accessor.Manifest()
	if manifest == "" {
		return nil, fmt.Errorf("render Helm chart %s: rendered manifest is empty", spec.Chart)
	}

	return []byte(manifest), nil
}

// Settings returns isolated Helm environment settings for this renderer.
func (r Renderer) Settings() (*cli.EnvSettings, error) {
	cacheDir := r.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "omni-cluster-operator", "helm")
	}
	cacheHome := filepath.Join(cacheDir, "cache")
	configHome := filepath.Join(cacheDir, "config")
	if err := os.MkdirAll(filepath.Join(cacheHome, "content"), 0755); err != nil {
		return nil, fmt.Errorf("create Helm cache directory %q: %w", cacheDir, err)
	}
	if err := os.MkdirAll(filepath.Join(cacheHome, "repository"), 0755); err != nil {
		return nil, fmt.Errorf("create Helm repository cache directory %q: %w", cacheDir, err)
	}
	if err := os.MkdirAll(filepath.Join(configHome, "registry"), 0755); err != nil {
		return nil, fmt.Errorf("create Helm config directory %q: %w", cacheDir, err)
	}

	settings := cli.New()
	settings.RepositoryCache = filepath.Join(cacheHome, "repository")
	settings.ContentCache = filepath.Join(cacheHome, "content")
	settings.RepositoryConfig = filepath.Join(configHome, "repositories.yaml")
	settings.RegistryConfig = filepath.Join(configHome, "registry", "config.json")

	return settings, nil
}
