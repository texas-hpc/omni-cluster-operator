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

package renderedmanifest

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	SecretKey      = "manifest.yaml"
	HashAnnotation = "omni.texashpc.com/rendered-manifest-hash"
)

type parseOptions struct {
	allowEmpty bool
}

// ParseOption configures rendered manifest parsing.
type ParseOption func(*parseOptions)

// AllowEmpty permits manifests that render no Kubernetes objects.
func AllowEmpty(options *parseOptions) {
	options.allowEmpty = true
}

// Hash returns a SHA-256 hash for rendered manifest bytes.
func Hash(manifest []byte) string {
	sum := sha256.Sum256(manifest)
	return hex.EncodeToString(sum[:])
}

// Parse converts a rendered multi-document YAML manifest into Omni inline JSON objects.
func Parse(manifest []byte, opts ...ParseOption) ([]apiextensionsv1.JSON, error) {
	options := parseOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	reader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(manifest)))
	var objects []apiextensionsv1.JSON

	for {
		doc, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}

			return nil, fmt.Errorf("read manifest document: %w", err)
		}

		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		rawJSON, err := sigsyaml.YAMLToJSON(doc)
		if err != nil {
			return nil, fmt.Errorf("convert manifest document to JSON: %w", err)
		}
		if string(bytes.TrimSpace(rawJSON)) == "null" {
			continue
		}

		var compact bytes.Buffer
		if err := json.Compact(&compact, rawJSON); err != nil {
			return nil, fmt.Errorf("compact manifest document JSON: %w", err)
		}
		if compact.Len() == 0 || compact.String() == "{}" {
			continue
		}

		objects = append(objects, apiextensionsv1.JSON{Raw: compact.Bytes()})
	}

	if len(objects) == 0 && !options.allowEmpty {
		return nil, fmt.Errorf("rendered manifest contains no Kubernetes objects")
	}

	return objects, nil
}

// SecretHasCurrentManifest reports whether a Secret already contains the desired render.
func SecretHasCurrentManifest(secret client.Object, data map[string][]byte, specHashAnnotation, specHash string) bool {
	annotations := secret.GetAnnotations()
	if annotations[specHashAnnotation] != specHash {
		return false
	}

	manifest, ok := data[SecretKey]
	if !ok {
		return false
	}

	return annotations[HashAnnotation] == Hash(manifest)
}
