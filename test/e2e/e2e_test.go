//go:build e2e
// +build e2e

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

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/texas-hpc/omni-cluster-operator/test/utils"
)

const namespace = "omni-cluster-operator-system"

var _ = Describe("Omni cluster operator", Ordered, func() {
	var controllerPodName string

	BeforeAll(func() {
		By("creating manager namespace")
		output, err := utils.Run(exec.Command("kubectl", "create", "namespace", namespace))
		if err != nil && !strings.Contains(output, "AlreadyExists") {
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace: %s", output)
		}

		By("labeling the namespace with restricted pod security")
		_, err = utils.Run(exec.Command("kubectl", "label", "--overwrite", "namespace", namespace,
			"pod-security.kubernetes.io/enforce=restricted"))
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace")

		By("installing CRDs")
		_, err = utils.Run(exec.Command("mise", "run", "install"))
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd := exec.Command("mise", "run", "deploy")
		cmd.Env = append(cmd.Environ(), fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy controller-manager")
	})

	AfterAll(func() {
		By("deleting Omni test resources")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"omniclusters,omnicontrolplanes,omniworkers,omnimachines,omniciliums,omniconnections",
			"--all", "-n", namespace, "--ignore-not-found", "--timeout=60s"))

		By("undeploying the controller-manager")
		cmd := exec.Command("mise", "run", "undeploy")
		cmd.Env = append(cmd.Environ(), "IGNORE_NOT_FOUND=true")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("mise", "run", "uninstall")
		cmd.Env = append(cmd.Environ(), "IGNORE_NOT_FOUND=true")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", namespace,
			"--ignore-not-found", "--timeout=60s"))
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}

		if controllerPodName != "" {
			By("fetching controller manager logs")
			output, err := utils.Run(exec.Command("kubectl", "logs", controllerPodName, "-n", namespace))
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "controller logs:\n%s\n", output)
			}
		}

		By("fetching namespace events")
		output, err := utils.Run(exec.Command("kubectl", "get", "events", "-n", namespace,
			"--sort-by=.lastTimestamp"))
		if err == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "events:\n%s\n", output)
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	It("runs the controller manager", func() {
		Eventually(func(g Gomega) {
			output, err := utils.Run(exec.Command("kubectl", "get",
				"pods", "-l", "control-plane=controller-manager",
				"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}{{ \"\\n\" }}{{ end }}{{ end }}",
				"-n", namespace))
			g.Expect(err).NotTo(HaveOccurred(), "Failed to list controller-manager pods")

			podNames := utils.GetNonEmptyLines(output)
			g.Expect(podNames).To(HaveLen(1), "expected one live controller-manager pod")
			controllerPodName = podNames[0]

			output, err = utils.Run(exec.Command("kubectl", "get", "pod", controllerPodName,
				"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}", "-n", namespace))
			g.Expect(err).NotTo(HaveOccurred(), "Failed to read controller-manager readiness")
			g.Expect(output).To(Equal("True"))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			_, err := kubectlApply(`
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniCluster
metadata:
  name: e2e-controller-ready
  namespace: omni-cluster-operator-system
spec:
  connectionRef:
    name: e2e-controller-ready
  kubernetes:
    version: v1.35.0
  talos:
    version: v1.13.2
  deletePolicy:
    orphan: true
  suspend: true
`)
			g.Expect(err).NotTo(HaveOccurred())
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			output, err := utils.Run(exec.Command("kubectl", "get", "omnicluster", "e2e-controller-ready",
				"-n", namespace, "-o", "jsonpath={.metadata.finalizers[0]}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("omni.texashpc.com/finalizer"))

			output, err = utils.Run(exec.Command("kubectl", "get", "omnicluster", "e2e-controller-ready",
				"-n", namespace, "-o", "jsonpath={.status.conditions[?(@.type=='Ready')].reason}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Suspended"))
		}).Should(Succeed())
	})

	It("rejects an invalid machine-set template shape", func() {
		output, err := kubectlApply(`
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniControlPlane
metadata:
  name: invalid-control-plane
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: invalid
  machines:
    - 11111111-1111-4111-8111-111111111111
  machineClass:
    name: control-plane
    size: 1
`)
		Expect(err).To(HaveOccurred(), "expected API server validation to reject invalid shape")
		Expect(output).To(ContainSubstring("exactly one of machines or machineClass is required"))

		Eventually(func(g Gomega) {
			output, err = kubectlApply(`
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniWorkers
metadata:
  name: invalid-workers
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: invalid
  workerSetName: control-planes
  machineClass:
    name: worker
    size: unlimited
`)
			g.Expect(err).To(HaveOccurred(), "expected validating webhook to reject reserved worker set name")
			g.Expect(output).To(ContainSubstring("workerSetName is reserved by Omni"))
		}).Should(Succeed())
	})

	It("reconciles a suspended cluster without contacting Omni", func() {
		_, err := kubectlApply(`
apiVersion: v1
kind: Secret
metadata:
  name: e2e-omni-service-account
  namespace: omni-cluster-operator-system
type: Opaque
stringData:
  serviceAccountKey: local-test-placeholder
---
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniConnection
metadata:
  name: e2e-omni
  namespace: omni-cluster-operator-system
spec:
  endpoint: https://omni.invalid.example
  auth:
    serviceAccountSecretRef:
      name: e2e-omni-service-account
---
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniCluster
metadata:
  name: e2e-suspended
  namespace: omni-cluster-operator-system
spec:
  connectionRef:
    name: e2e-omni
  kubernetes:
    version: v1.35.0
  talos:
    version: v1.13.2
  deletePolicy:
    orphan: true
  suspend: true
---
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniControlPlane
metadata:
  name: e2e-suspended-control-plane
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: e2e-suspended
  machineClass:
    name: control-plane
    size: 1
---
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniWorkers
metadata:
  name: e2e-suspended-workers
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: e2e-suspended
  machineClass:
    name: worker
    size: unlimited
`)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			output, err := utils.Run(exec.Command("kubectl", "get", "omnicluster", "e2e-suspended",
				"-n", namespace, "-o", "jsonpath={.metadata.finalizers[0]}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("omni.texashpc.com/finalizer"))

			output, err = utils.Run(exec.Command("kubectl", "get", "omnicluster", "e2e-suspended",
				"-n", namespace, "-o", "jsonpath={.status.conditions[?(@.type=='Ready')].reason}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Suspended"))
		}).Should(Succeed())
	})

	It("marks child template documents that reference a missing cluster", func() {
		_, err := kubectlApply(`
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniMachine
metadata:
  name: e2e-missing-cluster-machine
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: does-not-exist
  machineID: 33333333-3333-4333-8333-333333333333
`)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			output, err := utils.Run(exec.Command("kubectl", "get", "omnimachine",
				"e2e-missing-cluster-machine", "-n", namespace,
				"-o", "jsonpath={.status.conditions[?(@.type=='Accepted')].reason}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("MissingCluster"))
		}).Should(Succeed())
	})
})

func kubectlApply(manifest string) (string, error) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(strings.TrimSpace(manifest) + "\n")

	return utils.Run(cmd)
}
