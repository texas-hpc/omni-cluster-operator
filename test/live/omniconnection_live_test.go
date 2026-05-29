//go:build live_omni

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

package live

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	omniAPIGroup      = "omni.texas-hpc.org/v1alpha1"
	omniConnectionGVR = "omniconnections.omni.texas-hpc.org"
)

func TestLiveOmniConnection(t *testing.T) {
	endpoint, hasEndpoint := os.LookupEnv("OMNI_E2E_ENDPOINT")
	keyPath, hasKeyPath := os.LookupEnv("OMNI_E2E_SERVICE_ACCOUNT_KEY_FILE")
	if !hasEndpoint || strings.TrimSpace(endpoint) == "" || !hasKeyPath || strings.TrimSpace(keyPath) == "" {
		t.Skip("set OMNI_E2E_ENDPOINT and OMNI_E2E_SERVICE_ACCOUNT_KEY_FILE to run live Omni tests")
	}

	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read Omni service account key %q: %v", keyPath, err)
	}

	serviceAccountKey := strings.TrimSpace(string(keyBytes))
	if serviceAccountKey == "" {
		t.Fatalf("Omni service account key %q is empty", keyPath)
	}

	namespace := envOrDefault("LIVE_OMNI_OPERATOR_NAMESPACE", "omni-cluster-operator-system")
	name := fmt.Sprintf("live-omni-%d", time.Now().UnixNano()%1_000_000_000)
	secretName := name + "-service-account"

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	kubectl(ctx, t, "get", "namespace", namespace)

	defer kubectlAllowFailure(ctx, t, "delete", "-n", namespace, omniConnectionGVR, name, "--ignore-not-found")
	defer kubectlAllowFailure(ctx, t, "delete", "-n", namespace, "secret", secretName, "--ignore-not-found")

	applyJSON(ctx, t, map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      secretName,
			"namespace": namespace,
			"labels": map[string]string{
				"app.kubernetes.io/name":       "omni-cluster-operator",
				"app.kubernetes.io/created-by": "live-omni-test",
			},
		},
		"type": "Opaque",
		"stringData": map[string]string{
			"serviceAccountKey": serviceAccountKey,
		},
	})

	applyJSON(ctx, t, map[string]any{
		"apiVersion": omniAPIGroup,
		"kind":       "OmniConnection",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]string{
				"app.kubernetes.io/name":       "omni-cluster-operator",
				"app.kubernetes.io/created-by": "live-omni-test",
			},
		},
		"spec": map[string]any{
			"endpoint":              endpoint,
			"insecureSkipTLSVerify": envBool("OMNI_E2E_INSECURE_SKIP_TLS_VERIFY"),
			"auth": map[string]any{
				"serviceAccountSecretRef": map[string]string{
					"name": secretName,
					"key":  "serviceAccountKey",
				},
			},
		},
	})

	if _, err := kubectlOutput(ctx, "wait", "-n", namespace, omniConnectionGVR+"/"+name, "--for=condition=Ready", "--timeout=3m"); err != nil {
		t.Logf("OmniConnection after failed wait:\n%s", kubectlAllowFailure(ctx, t, "get", "-n", namespace, omniConnectionGVR, name, "-o", "yaml"))
		t.Fatalf("wait for OmniConnection Ready: %v", err)
	}

	status := kubectl(ctx, t, "get", "-n", namespace, omniConnectionGVR, name, "-o", "jsonpath={.status.conditions[?(@.type=='Ready')].message}")
	t.Logf("OmniConnection Ready: %s", strings.TrimSpace(status))
}

func applyJSON(ctx context.Context, t *testing.T, object map[string]any) {
	t.Helper()

	payload, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal object: %v", err)
	}

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader(payload)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl apply failed: %v\n%s", err, string(output))
	}
}

func kubectl(ctx context.Context, t *testing.T, args ...string) string {
	t.Helper()

	output, err := kubectlOutput(ctx, args...)
	if err != nil {
		t.Fatalf("kubectl %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}

	return output
}

func kubectlAllowFailure(ctx context.Context, t *testing.T, args ...string) string {
	t.Helper()

	output, _ := kubectlOutput(ctx, args...)

	return output
}

func kubectlOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()

	return string(output), err
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	return value
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "t", "true", "y", "yes":
		return true
	default:
		return false
	}
}
