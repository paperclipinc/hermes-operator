package resources

import (
	"crypto/sha256"
	"encoding/hex"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

// apiServerKeyFor derives a stable, per-instance API server key. Deterministic
// from the instance UID so it never thrashes across reconciles. NOTE: derivable
// by anyone who can read the HermesInstance UID — a controller-generated random
// key with read-preserve semantics is the hardening follow-up; the gateway API
// is additionally fronted by the default-deny NetworkPolicy.
func apiServerKeyFor(inst *hermesv1.HermesInstance) string {
	sum := sha256.Sum256([]byte(string(inst.UID) + ":hermes-api-server-key"))
	return hex.EncodeToString(sum[:])
}

// GatewayTokenSecretName returns the deterministic name for the operator-owned
// gateway-tokens Secret.
func GatewayTokenSecretName(inst *hermesv1.HermesInstance) string {
	return inst.Name + "-gateway-tokens"
}

// BuildGatewayTokenSecret returns a placeholder Secret owned by the instance.
// Plan 2 emits an empty Secret with the "hermes.agent/placeholder: true"
// annotation; Plan 3 replaces the body with gateway-token bytes resolved from
// spec.gateways.*.tokenSecretRef. Until Plan 3 lands, the agent reads its tokens
// from user-provided EnvFrom secrets directly.
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
		Data: map[string][]byte{
			APIServerKeySecretKey: []byte(apiServerKeyFor(inst)),
		},
	}
}
