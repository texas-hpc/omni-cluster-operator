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

package omnitemplate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	omnioperations "github.com/siderolabs/omni/client/pkg/template/operations"
	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"go.yaml.in/yaml/v4"
)

// Inputs are the Kubernetes CRs that make up one Omni cluster template.
type Inputs struct {
	Cluster      *omniv1alpha1.OmniCluster
	ControlPlane *omniv1alpha1.OmniControlPlane
	Workers      []omniv1alpha1.OmniWorkers
	Machines     []omniv1alpha1.OmniMachine
}

// Result is a rendered Omni cluster template and metadata derived from it.
type Result struct {
	ClusterName     string
	Template        []byte
	TemplateHash    string
	ControlPlaneRef string
	WorkersRefs     []string
	MachineRefs     []string
}

type clusterDoc struct {
	Kind             string            `yaml:"kind"`
	Name             string            `yaml:"name"`
	Labels           map[string]string `yaml:"labels,omitempty"`
	Annotations      map[string]string `yaml:"annotations,omitempty"`
	Kubernetes       kubernetesDoc     `yaml:"kubernetes"`
	Talos            talosDoc          `yaml:"talos"`
	Features         *featuresDoc      `yaml:"features,omitempty"`
	Patches          []patchDoc        `yaml:"patches,omitempty"`
	SystemExtensions []string          `yaml:"systemExtensions,omitempty"`
	KernelArgs       []string          `yaml:"kernelArgs,omitempty"`
}

type controlPlaneDoc struct {
	Kind             string            `yaml:"kind"`
	Labels           map[string]string `yaml:"labels,omitempty"`
	Annotations      map[string]string `yaml:"annotations,omitempty"`
	Machines         []string          `yaml:"machines,omitempty"`
	MachineClass     *machineClassDoc  `yaml:"machineClass,omitempty"`
	BootstrapSpec    *bootstrapDoc     `yaml:"bootstrapSpec,omitempty"`
	Patches          []patchDoc        `yaml:"patches,omitempty"`
	SystemExtensions []string          `yaml:"systemExtensions,omitempty"`
	KernelArgs       []string          `yaml:"kernelArgs,omitempty"`
}

type workersDoc struct {
	Kind             string             `yaml:"kind"`
	Name             string             `yaml:"name,omitempty"`
	Labels           map[string]string  `yaml:"labels,omitempty"`
	Annotations      map[string]string  `yaml:"annotations,omitempty"`
	Machines         []string           `yaml:"machines,omitempty"`
	MachineClass     *machineClassDoc   `yaml:"machineClass,omitempty"`
	UpdateStrategy   *updateStrategyDoc `yaml:"updateStrategy,omitempty"`
	UpgradeStrategy  *updateStrategyDoc `yaml:"upgradeStrategy,omitempty"`
	DeleteStrategy   *updateStrategyDoc `yaml:"deleteStrategy,omitempty"`
	Patches          []patchDoc         `yaml:"patches,omitempty"`
	SystemExtensions []string           `yaml:"systemExtensions,omitempty"`
	KernelArgs       []string           `yaml:"kernelArgs,omitempty"`
}

type machineDoc struct {
	Kind             string            `yaml:"kind"`
	Name             string            `yaml:"name"`
	Labels           map[string]string `yaml:"labels,omitempty"`
	Annotations      map[string]string `yaml:"annotations,omitempty"`
	Locked           bool              `yaml:"locked,omitempty"`
	Install          *installDoc       `yaml:"install,omitempty"`
	Patches          []patchDoc        `yaml:"patches,omitempty"`
	SystemExtensions []string          `yaml:"systemExtensions,omitempty"`
	KernelArgs       []string          `yaml:"kernelArgs,omitempty"`
}

type kubernetesDoc struct {
	Version  string        `yaml:"version"`
	Manifest []manifestDoc `yaml:"manifests,omitempty"`
}

type talosDoc struct {
	Version string `yaml:"version"`
}

type featuresDoc struct {
	EnableWorkloadProxy         bool             `yaml:"enableWorkloadProxy,omitempty"`
	UseEmbeddedDiscoveryService bool             `yaml:"useEmbeddedDiscoveryService,omitempty"`
	DiskEncryption              bool             `yaml:"diskEncryption,omitempty"`
	BackupConfiguration         *backupConfigDoc `yaml:"backupConfiguration,omitempty"`
}

type backupConfigDoc struct {
	Interval string `yaml:"interval"`
}

type manifestDoc struct {
	Name   string `yaml:"name"`
	Mode   string `yaml:"mode"`
	Inline []any  `yaml:"inline,omitempty"`
	File   string `yaml:"file,omitempty"`
}

type patchDoc struct {
	File        string            `yaml:"file,omitempty"`
	Name        string            `yaml:"name,omitempty"`
	IDOverride  string            `yaml:"idOverride,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
	Inline      any               `yaml:"inline,omitempty"`
}

type machineClassDoc struct {
	Name string `yaml:"name"`
	Size any    `yaml:"size"`
}

type bootstrapDoc struct {
	ClusterUUID string `yaml:"clusterUUID"`
	Snapshot    string `yaml:"snapshot"`
}

type updateStrategyDoc struct {
	Type    string      `yaml:"type,omitempty"`
	Rolling *rollingDoc `yaml:"rolling,omitempty"`
}

type rollingDoc struct {
	MaxParallelism *int32 `yaml:"maxParallelism,omitempty"`
}

type installDoc struct {
	Disk string `yaml:"disk"`
}

// Render renders a complete Omni multi-document cluster template.
func Render(in Inputs) (*Result, error) {
	if in.Cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}

	if in.ControlPlane == nil {
		return nil, fmt.Errorf("exactly one OmniControlPlane must reference OmniCluster %q", in.Cluster.Name)
	}

	workers := append([]omniv1alpha1.OmniWorkers(nil), in.Workers...)
	sort.Slice(workers, func(i, j int) bool {
		return workerSetName(&workers[i]) < workerSetName(&workers[j])
	})

	machines := append([]omniv1alpha1.OmniMachine(nil), in.Machines...)
	sort.Slice(machines, func(i, j int) bool {
		return machineID(&machines[i]) < machineID(&machines[j])
	})

	clusterName := ClusterName(in.Cluster)
	docs := []any{renderCluster(in.Cluster, clusterName), renderControlPlane(in.ControlPlane)}
	result := &Result{
		ClusterName:     clusterName,
		ControlPlaneRef: in.ControlPlane.Name,
	}

	for i := range workers {
		name := workerSetName(&workers[i])
		if name == "control-planes" {
			return nil, fmt.Errorf("workers %q uses reserved workerSetName %q", workers[i].Name, name)
		}

		docs = append(docs, renderWorkers(&workers[i], name))
		result.WorkersRefs = append(result.WorkersRefs, workers[i].Name)
	}

	for i := range machines {
		docs = append(docs, renderMachine(&machines[i], machineID(&machines[i])))
		result.MachineRefs = append(result.MachineRefs, machines[i].Name)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	for _, doc := range docs {
		if err := enc.Encode(doc); err != nil {
			return nil, fmt.Errorf("encode template document: %w", err)
		}
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close template encoder: %w", err)
	}

	sum := sha256.Sum256(buf.Bytes())
	result.Template = buf.Bytes()
	result.TemplateHash = hex.EncodeToString(sum[:])

	return result, nil
}

// Validate validates rendered template bytes with Omni's template validator.
func Validate(templateBytes []byte, rootPath string) error {
	root, closeRoot, err := OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer closeRoot()

	return omnioperations.ValidateTemplate(bytes.NewReader(templateBytes), root)
}

// OpenRoot opens a restricted template file root for Omni file-based template fields.
func OpenRoot(rootPath string) (*os.Root, func(), error) {
	if rootPath == "" {
		return nil, func() {}, nil
	}

	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open template root %q: %w", rootPath, err)
	}

	return root, func() { _ = root.Close() }, nil
}

// ClusterName returns the Omni cluster name for a CR.
func ClusterName(cluster *omniv1alpha1.OmniCluster) string {
	if cluster.Spec.ClusterName != "" {
		return cluster.Spec.ClusterName
	}

	return cluster.Name
}

func renderCluster(cluster *omniv1alpha1.OmniCluster, clusterName string) clusterDoc {
	return clusterDoc{
		Kind:        "Cluster",
		Name:        clusterName,
		Labels:      cluster.Spec.Labels,
		Annotations: cluster.Spec.Annotations,
		Kubernetes: kubernetesDoc{
			Version:  cluster.Spec.Kubernetes.Version,
			Manifest: renderManifests(cluster.Spec.Kubernetes.Manifests),
		},
		Talos:            talosDoc{Version: cluster.Spec.Talos.Version},
		Features:         renderFeatures(cluster.Spec.Features),
		Patches:          renderPatches(cluster.Spec.Patches),
		SystemExtensions: cluster.Spec.SystemExtensions,
		KernelArgs:       cluster.Spec.KernelArgs,
	}
}

func renderControlPlane(controlPlane *omniv1alpha1.OmniControlPlane) controlPlaneDoc {
	spec := controlPlane.Spec

	return controlPlaneDoc{
		Kind:             "ControlPlane",
		Labels:           spec.Labels,
		Annotations:      spec.Annotations,
		Machines:         spec.Machines,
		MachineClass:     renderMachineClass(spec.MachineClass),
		BootstrapSpec:    renderBootstrap(spec.BootstrapSpec),
		Patches:          renderPatches(spec.Patches),
		SystemExtensions: spec.SystemExtensions,
		KernelArgs:       spec.KernelArgs,
	}
}

func renderWorkers(workers *omniv1alpha1.OmniWorkers, name string) workersDoc {
	spec := workers.Spec

	return workersDoc{
		Kind:             "Workers",
		Name:             name,
		Labels:           spec.Labels,
		Annotations:      spec.Annotations,
		Machines:         spec.Machines,
		MachineClass:     renderMachineClass(spec.MachineClass),
		UpdateStrategy:   renderUpdateStrategy(spec.UpdateStrategy),
		UpgradeStrategy:  renderUpdateStrategy(spec.UpgradeStrategy),
		DeleteStrategy:   renderUpdateStrategy(spec.DeleteStrategy),
		Patches:          renderPatches(spec.Patches),
		SystemExtensions: spec.SystemExtensions,
		KernelArgs:       spec.KernelArgs,
	}
}

func renderMachine(machine *omniv1alpha1.OmniMachine, id string) machineDoc {
	spec := machine.Spec

	return machineDoc{
		Kind:             "Machine",
		Name:             id,
		Labels:           spec.Labels,
		Annotations:      spec.Annotations,
		Locked:           spec.Locked,
		Install:          renderInstall(spec.Install),
		Patches:          renderPatches(spec.Patches),
		SystemExtensions: spec.SystemExtensions,
		KernelArgs:       spec.KernelArgs,
	}
}

func renderFeatures(features *omniv1alpha1.ClusterFeatures) *featuresDoc {
	if features == nil {
		return nil
	}

	var backup *backupConfigDoc
	if features.BackupConfiguration != nil {
		backup = &backupConfigDoc{Interval: features.BackupConfiguration.Interval}
	}

	return &featuresDoc{
		EnableWorkloadProxy:         features.EnableWorkloadProxy,
		UseEmbeddedDiscoveryService: features.UseEmbeddedDiscoveryService,
		DiskEncryption:              features.DiskEncryption,
		BackupConfiguration:         backup,
	}
}

func renderManifests(manifests []omniv1alpha1.KubernetesManifest) []manifestDoc {
	if len(manifests) == 0 {
		return nil
	}

	rendered := make([]manifestDoc, 0, len(manifests))
	for _, manifest := range manifests {
		rendered = append(rendered, manifestDoc{
			Name:   manifest.Name,
			Mode:   manifest.Mode,
			Inline: renderJSONList(manifest.Inline),
			File:   manifest.File,
		})
	}

	return rendered
}

func renderPatches(patches []omniv1alpha1.Patch) []patchDoc {
	if len(patches) == 0 {
		return nil
	}

	rendered := make([]patchDoc, 0, len(patches))
	for _, patch := range patches {
		var inline any
		if patch.Inline != nil {
			inline = renderJSON(*patch.Inline)
		}

		rendered = append(rendered, patchDoc{
			File:        patch.File,
			Name:        patch.Name,
			IDOverride:  patch.IDOverride,
			Labels:      patch.Labels,
			Annotations: patch.Annotations,
			Inline:      inline,
		})
	}

	return rendered
}

func renderMachineClass(machineClass *omniv1alpha1.MachineClass) *machineClassDoc {
	if machineClass == nil {
		return nil
	}

	return &machineClassDoc{
		Name: machineClass.Name,
		Size: intOrStringValue(machineClass.Size),
	}
}

func renderBootstrap(bootstrap *omniv1alpha1.BootstrapSpec) *bootstrapDoc {
	if bootstrap == nil {
		return nil
	}

	return &bootstrapDoc{
		ClusterUUID: bootstrap.ClusterUUID,
		Snapshot:    bootstrap.Snapshot,
	}
}

func renderUpdateStrategy(strategy *omniv1alpha1.UpdateStrategy) *updateStrategyDoc {
	if strategy == nil {
		return nil
	}

	var rolling *rollingDoc
	if strategy.Rolling != nil {
		rolling = &rollingDoc{MaxParallelism: strategy.Rolling.MaxParallelism}
	}

	return &updateStrategyDoc{
		Type:    strategy.Type,
		Rolling: rolling,
	}
}

func renderInstall(install *omniv1alpha1.MachineInstallSpec) *installDoc {
	if install == nil {
		return nil
	}

	return &installDoc{Disk: install.Disk}
}

func workerSetName(workers *omniv1alpha1.OmniWorkers) string {
	if workers.Spec.WorkerSetName != "" {
		return workers.Spec.WorkerSetName
	}

	return workers.Name
}

func machineID(machine *omniv1alpha1.OmniMachine) string {
	if machine.Spec.MachineID != "" {
		return machine.Spec.MachineID
	}

	return machine.Name
}

func intOrStringValue(value intstr.IntOrString) any {
	if value.Type == intstr.String {
		return value.StrVal
	}

	return value.IntVal
}

func renderJSONList(values []apiextensionsv1.JSON) []any {
	if len(values) == 0 {
		return nil
	}

	rendered := make([]any, 0, len(values))
	for _, value := range values {
		rendered = append(rendered, renderJSON(value))
	}

	return rendered
}

func renderJSON(value apiextensionsv1.JSON) any {
	if len(value.Raw) == 0 {
		return map[string]any{}
	}

	var decoded any
	if err := json.Unmarshal(value.Raw, &decoded); err != nil {
		return string(value.Raw)
	}

	return decoded
}
