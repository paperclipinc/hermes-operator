package resources

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

// APIServerKeySecretKey is the key under the operator-managed gateway-tokens
// Secret that holds the OpenAI-compatible API server auth key.
const APIServerKeySecretKey = "api_server_key"

// BuildAgentRuntimeEnv returns the environment that drives the upstream s6
// hermes-agent image so a HermesInstance comes up as a long-lived gateway with
// the OpenAI-compatible API server and a /health endpoint on the gateway port:
//
//   - HERMES_HOME points the agent at the persistent /opt/data volume.
//   - HERMES_UID/HERMES_GID drive the s6 stage2 privilege drop to a non-root user
//     (the container itself starts as root so s6 can remap + chown the volume).
//   - API_SERVER_* enables the gateway's OpenAI-compatible HTTP API and /health
//     on the gateway port. The key authenticates /v1; /health is unauthenticated,
//     which is what the readiness/liveness probes hit.
func BuildAgentRuntimeEnv(inst *hermesv1.HermesInstance) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "HERMES_HOME", Value: "/opt/data"},
		{Name: "HERMES_UID", Value: "1000"},
		{Name: "HERMES_GID", Value: "1000"},
		{Name: "API_SERVER_ENABLED", Value: "true"},
		{Name: "API_SERVER_HOST", Value: "0.0.0.0"},
		{Name: "API_SERVER_PORT", Value: strconv.Itoa(int(GatewayPort))},
		{
			Name: "API_SERVER_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: GatewayTokenSecretName(inst),
					},
					Key: APIServerKeySecretKey,
				},
			},
		},
	}
}
