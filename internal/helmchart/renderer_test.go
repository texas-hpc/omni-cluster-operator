package helmchart

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRendererSettingsUseIsolatedHelmPaths(t *testing.T) {
	cacheDir := t.TempDir()

	settings, err := Renderer{CacheDir: cacheDir}.Settings()
	if err != nil {
		t.Fatalf("Settings() error = %v", err)
	}

	wantRepositoryCache := filepath.Join(cacheDir, "cache", "repository")
	wantContentCache := filepath.Join(cacheDir, "cache", "content")
	wantRepositoryConfig := filepath.Join(cacheDir, "config", "repositories.yaml")
	wantRegistryConfig := filepath.Join(cacheDir, "config", "registry", "config.json")

	if settings.RepositoryCache != wantRepositoryCache {
		t.Fatalf("RepositoryCache = %q, want %q", settings.RepositoryCache, wantRepositoryCache)
	}
	if settings.ContentCache != wantContentCache {
		t.Fatalf("ContentCache = %q, want %q", settings.ContentCache, wantContentCache)
	}
	if settings.RepositoryConfig != wantRepositoryConfig {
		t.Fatalf("RepositoryConfig = %q, want %q", settings.RepositoryConfig, wantRepositoryConfig)
	}
	if settings.RegistryConfig != wantRegistryConfig {
		t.Fatalf("RegistryConfig = %q, want %q", settings.RegistryConfig, wantRegistryConfig)
	}

	for _, dir := range []string{
		wantRepositoryCache,
		wantContentCache,
		filepath.Dir(wantRepositoryConfig),
		filepath.Dir(wantRegistryConfig),
	} {
		if info, err := os.Stat(dir); err != nil {
			t.Fatalf("expected directory %q to exist: %v", dir, err)
		} else if !info.IsDir() {
			t.Fatalf("expected %q to be a directory", dir)
		}
	}
}
