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

package helmrelease

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/registry"
	helmrelease "helm.sh/helm/v4/pkg/release"
	"helm.sh/helm/v4/pkg/storage/driver"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/helmchart"
)

// Client reconciles Helm releases directly against a workload cluster.
type Client struct {
	CacheDir string
}

var locateChartMu sync.Mutex

// Reconcile installs or upgrades the desired Helm release.
func (c Client) Reconcile(ctx context.Context, item *omniv1alpha1.OmniHelmRelease, kubeconfig []byte) (*Result, error) {
	values, err := Values(item)
	if err != nil {
		return nil, err
	}

	cfg, settings, err := c.actionConfig(item, kubeconfig)
	if err != nil {
		return nil, err
	}

	chart, err := c.loadChart(ctx, item, cfg, settings)
	if err != nil {
		return nil, err
	}

	releaseName := ReleaseName(item)
	if _, err := cfg.Releases.Last(releaseName); errors.Is(err, driver.ErrReleaseNotFound) {
		return c.install(ctx, item, cfg, chart, values)
	} else if err != nil {
		return nil, fmt.Errorf("read Helm release history for %q: %w", releaseName, err)
	}

	return c.upgrade(ctx, item, cfg, chart, values)
}

// Uninstall uninstalls the Helm release from the workload cluster.
func (c Client) Uninstall(ctx context.Context, item *omniv1alpha1.OmniHelmRelease, kubeconfig []byte) (*Result, error) {
	cfg, _, err := c.actionConfig(item, kubeconfig)
	if err != nil {
		return nil, err
	}

	client := action.NewUninstall(cfg)
	client.DisableHooks = item.Spec.DisableHooks
	client.IgnoreNotFound = true
	client.Timeout = Timeout(item)
	client.WaitStrategy = waitStrategy(item)

	response, err := client.Run(ReleaseName(item))
	result := &Result{
		Action:       ActionUninstall,
		ReleaseName:  ReleaseName(item),
		Namespace:    Namespace(item),
		Chart:        item.Spec.Chart.Chart,
		ChartVersion: item.Spec.Chart.Version,
		Status:       StatusUninstalled,
	}
	if response != nil && response.Release != nil {
		accessor, accessorErr := helmrelease.NewAccessor(response.Release)
		if accessorErr != nil {
			return result, fmt.Errorf("read uninstalled Helm release: %w", accessorErr)
		}
		result.Revision = accessor.Version()
		result.Status = accessor.Status()
	}

	return result, err
}

func (c Client) install(ctx context.Context, item *omniv1alpha1.OmniHelmRelease, cfg *action.Configuration, chart any, values map[string]any) (*Result, error) {
	client := action.NewInstall(cfg)
	client.ReleaseName = ReleaseName(item)
	client.Namespace = Namespace(item)
	client.CreateNamespace = item.Spec.CreateNamespace
	client.DisableHooks = item.Spec.DisableHooks
	client.SkipCRDs = item.Spec.SkipCRDs
	client.WaitStrategy = waitStrategy(item)
	client.WaitForJobs = item.Spec.WaitForJobs
	client.Timeout = Timeout(item)
	client.RollbackOnFailure = item.Spec.Atomic

	release, err := client.RunWithContext(ctx, chart, values)

	return resultFromRelease(ActionInstall, item, release), err
}

func (c Client) upgrade(ctx context.Context, item *omniv1alpha1.OmniHelmRelease, cfg *action.Configuration, chart any, values map[string]any) (*Result, error) {
	client := action.NewUpgrade(cfg)
	client.Namespace = Namespace(item)
	client.DisableHooks = item.Spec.DisableHooks
	client.SkipCRDs = item.Spec.SkipCRDs
	client.WaitStrategy = waitStrategy(item)
	client.WaitForJobs = item.Spec.WaitForJobs
	client.Timeout = Timeout(item)
	client.RollbackOnFailure = item.Spec.Atomic
	client.CleanupOnFail = item.Spec.Atomic
	client.MaxHistory = item.Spec.MaxHistory

	release, err := client.RunWithContext(ctx, ReleaseName(item), chart, values)

	return resultFromRelease(ActionUpgrade, item, release), err
}

func (c Client) actionConfig(item *omniv1alpha1.OmniHelmRelease, kubeconfig []byte) (*action.Configuration, *cli.EnvSettings, error) {
	getter, err := newRESTClientGetter(kubeconfig)
	if err != nil {
		return nil, nil, err
	}

	settings, err := helmchart.Renderer{CacheDir: c.CacheDir}.Settings()
	if err != nil {
		return nil, nil, err
	}

	cfg := action.NewConfiguration()
	if err := cfg.Init(getter, Namespace(item), "secrets"); err != nil {
		return nil, nil, fmt.Errorf("initialize Helm action configuration: %w", err)
	}

	registryClient, err := registry.NewClient(
		registry.ClientOptDebug(settings.Debug),
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptCredentialsFile(settings.RegistryConfig),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize Helm registry client: %w", err)
	}
	cfg.RegistryClient = registryClient

	return cfg, settings, nil
}

func (c Client) loadChart(ctx context.Context, item *omniv1alpha1.OmniHelmRelease, cfg *action.Configuration, settings *cli.EnvSettings) (any, error) {
	client := action.NewInstall(cfg)
	chartRef, repoURL := ChartLocator(item)
	client.RepoURL = repoURL
	client.Version = item.Spec.Chart.Version
	client.ReleaseName = ReleaseName(item)
	client.Namespace = Namespace(item)

	locateChartMu.Lock()
	chartPath, err := client.LocateChart(chartRef, settings)
	locateChartMu.Unlock()
	if err != nil {
		if repoURL == "" {
			return nil, fmt.Errorf("locate Helm chart %s %s: %w", chartRef, item.Spec.Chart.Version, err)
		}

		return nil, fmt.Errorf("locate Helm chart %s %s from %s: %w", chartRef, item.Spec.Chart.Version, repoURL, err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("load Helm chart %q: %w", chartPath, err)
	}

	return chart, nil
}

func waitStrategy(item *omniv1alpha1.OmniHelmRelease) kube.WaitStrategy {
	if item.Spec.Wait {
		return kube.StatusWatcherStrategy
	}

	return kube.HookOnlyStrategy
}

func resultFromRelease(actionName string, item *omniv1alpha1.OmniHelmRelease, release any) *Result {
	result := &Result{
		Action:       actionName,
		ReleaseName:  ReleaseName(item),
		Namespace:    Namespace(item),
		Chart:        item.Spec.Chart.Chart,
		ChartVersion: item.Spec.Chart.Version,
		Status:       StatusUnknown,
	}
	if release == nil {
		return result
	}

	accessor, err := helmrelease.NewAccessor(release)
	if err != nil {
		return result
	}
	result.ReleaseName = accessor.Name()
	result.Namespace = accessor.Namespace()
	result.Revision = accessor.Version()
	result.Status = accessor.Status()

	return result
}
