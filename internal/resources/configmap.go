package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hermesv1 "github.com/stubbi/hermes-operator/api/v1"
)

// ConfigMapName returns the deterministic ConfigMap name for a HermesInstance.
func ConfigMapName(inst *hermesv1.HermesInstance) string {
	return inst.Name + "-config"
}

// BuildConfigMap returns the desired ConfigMap holding ~/.hermes/config.yaml.
// In this plan the body is a minimal empty mapping. Plan 2 wires spec.config.
func BuildConfigMap(inst *hermesv1.HermesInstance) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(inst),
			Namespace: inst.Namespace,
			Labels:    LabelsForInstance(inst),
		},
		Data: map[string]string{
			"config.yaml": "{}\n",
		},
	}
}
