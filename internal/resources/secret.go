package resources

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

// GatewayTokenSecretName returns the deterministic name for the operator-owned
// gateway-tokens Secret.
func GatewayTokenSecretName(inst *hermesv1.HermesInstance) string {
	return inst.Name + "-gateway-tokens"
}

// GenerateAPIServerKey returns a cryptographically random key for the agent's
// OpenAI-compatible API server. The reconciler generates this once on Secret
// creation and preserves it across reconciles (see reconcileSecret), so it never
// thrashes and is never derivable from public object metadata.
func GenerateAPIServerKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api server key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// BuildGatewayTokenSecret returns the operator-owned per-instance Secret. It is a
// pure builder: the value of the API server key (APIServerKeySecretKey) is filled
// in and preserved by the reconciler, not here, so the builder stays
// deterministic. Plan 3 will additionally populate gateway-token bytes resolved
// from spec.gateways.*.tokenSecretRef.
func BuildGatewayTokenSecret(inst *hermesv1.HermesInstance) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayTokenSecretName(inst),
			Namespace: inst.Namespace,
			Labels:    LabelsForInstance(inst),
			Annotations: map[string]string{
				"hermes.agent/placeholder": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}
}
