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
	"fmt"
	"os"
	"path/filepath"

	"helm.sh/helm/v4/pkg/cli"
)

// Renderer provides isolated Helm environment settings.
type Renderer struct {
	CacheDir string
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
