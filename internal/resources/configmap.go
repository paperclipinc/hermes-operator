package resources

import (
	"fmt"

	hermesv1 "github.com/stubbi/hermes-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// ConfigMapName returns the deterministic ConfigMap name for the rendered config.
func ConfigMapName(inst *hermesv1.HermesInstance) string {
	return inst.Name + "-config"
}

// BuildConfigMap returns the desired ConfigMap holding ~/.hermes/config.yaml.
//
// `resolvedBody` is the body the reconciler has already resolved for the case
// where spec.config.configMapRef is set. The builder is pure — it does not
// reach out to the apiserver.
//
//   - Empty resolvedBody + Raw set         → use Raw verbatim (YAML-serialised).
//   - Empty resolvedBody + Raw unset       → emit "{}\n".
//   - resolvedBody non-empty + Raw unset   → use resolvedBody verbatim.
//   - resolvedBody non-empty + Raw set     → caller is responsible for merging
//     (use MergeYAMLBodies) and passing the merged result as resolvedBody.
func BuildConfigMap(inst *hermesv1.HermesInstance, resolvedBody string) *corev1.ConfigMap {
	body := "{}\n"
	switch {
	case resolvedBody != "":
		body = resolvedBody
	case inst.Spec.Config.Raw != nil && len(inst.Spec.Config.Raw.Raw) > 0:
		y, err := yaml.JSONToYAML(inst.Spec.Config.Raw.Raw)
		if err == nil {
			body = string(y)
		}
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(inst),
			Namespace: inst.Namespace,
			Labels:    LabelsForInstance(inst),
		},
		Data: map[string]string{
			"config.yaml": body,
		},
	}
}

// MergeYAMLBodies performs a YAML deep-merge of `overlay` (JSON or YAML) onto
// `base` (YAML). Overlay wins on conflict. Used when spec.config.mergeMode=merge.
func MergeYAMLBodies(base, overlay string) (string, error) {
	baseMap := map[string]any{}
	if base != "" {
		if err := yaml.Unmarshal([]byte(base), &baseMap); err != nil {
			return "", fmt.Errorf("parse base YAML: %w", err)
		}
	}
	overlayMap := map[string]any{}
	if overlay != "" {
		if err := yaml.Unmarshal([]byte(overlay), &overlayMap); err != nil {
			return "", fmt.Errorf("parse overlay: %w", err)
		}
	}
	merged := deepMergeMaps(baseMap, overlayMap)
	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("marshal merged: %w", err)
	}
	return string(out), nil
}

func deepMergeMaps(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if bv, ok := out[k]; ok {
			bm, bok := bv.(map[string]any)
			vm, vok := v.(map[string]any)
			if bok && vok {
				out[k] = deepMergeMaps(bm, vm)
				continue
			}
		}
		out[k] = v
	}
	return out
}
