package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

const (
	tailscaleContainerName = "tailscale"
	tailscaleServeMount    = "/etc/tailscale"
	tailscaleServeFile     = "serve.json"
	tailscaleServeVolume   = "tailscale-serve"
	tailscaleServeKey      = "tailscale-serve.json"
	tailscaleTmpVolume     = "tailscale-tmp"

	tailscaleDefaultRepository = "tailscale/tailscale"
	tailscaleDefaultTag        = "v1.86.2"
)

func tailscaleEnabled(inst *hermesv1.HermesInstance) bool {
	return BoolValue(inst.Spec.Tailscale.Enabled)
}

func tailscaleImage(inst *hermesv1.HermesInstance) string {
	repo := inst.Spec.Tailscale.Image.Repository
	if repo == "" {
		repo = tailscaleDefaultRepository
	}
	tag := inst.Spec.Tailscale.Image.Tag
	if tag == "" {
		tag = tailscaleDefaultTag
	}
	return fmt.Sprintf("%s:%s", repo, tag)
}

func tailscaleHostname(inst *hermesv1.HermesInstance) string {
	if inst.Spec.Tailscale.Hostname != "" {
		return inst.Spec.Tailscale.Hostname
	}
	return inst.Name
}

// BuildTailscaleServeConfig renders the Tailscale Serve config JSON that fronts
// the local hermes gateway (127.0.0.1:<GatewayPort>) on tailnet :443.
//
// Spec.Tailscale.Mode is intentionally not read here: the CRD enum admits only
// "serve", so every enabled instance gets this serve mapping. Note that Mode
// can also be "" (CRD defaulting only materializes when the tailscale key is
// present, and unit tests build specs directly); "" must behave like "serve".
// If a second mode is ever added to the enum, this function is where the
// behavior must branch.
func BuildTailscaleServeConfig(_ *hermesv1.HermesInstance) string {
	return fmt.Sprintf(`{
  "TCP": { "443": { "HTTPS": true } },
  "Web": {
    "${TS_CERT_DOMAIN}:443": {
      "Handlers": { "/": { "Proxy": "http://127.0.0.1:%d" } }
    }
  }
}`, GatewayPort)
}

// BuildTailscaleSidecar builds the operator-managed tailscale sidecar container,
// or nil when tailscale is disabled.
func BuildTailscaleSidecar(inst *hermesv1.HermesInstance) *corev1.Container {
	if !tailscaleEnabled(inst) {
		return nil
	}
	ts := inst.Spec.Tailscale

	pullPolicy := corev1.PullPolicy(ts.Image.PullPolicy)
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}

	// No TS_KUBE_SECRET and no TS_STATE_DIR: containerboot then defaults to
	// `--state=mem: --statedir=/tmp`, i.e. in-memory ephemeral state.
	// Ephemerality itself comes from the auth key, which the user supplies as
	// reusable + ephemeral.
	env := []corev1.EnvVar{
		{Name: "TS_USERSPACE", Value: "true"},
		{Name: "TS_HOSTNAME", Value: tailscaleHostname(inst)},
		{Name: "TS_SERVE_CONFIG", Value: tailscaleServeMount + "/" + tailscaleServeFile},
	}
	if ts.AuthKey != nil && ts.AuthKey.SecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name: "TS_AUTHKEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: ts.AuthKey.SecretRef,
			},
		})
	}

	return &corev1.Container{
		Name:            tailscaleContainerName,
		Image:           tailscaleImage(inst),
		ImagePullPolicy: pullPolicy,
		Env:             env,
		Resources:       ts.Resources,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      tailscaleServeVolume,
				MountPath: tailscaleServeMount,
				ReadOnly:  true,
			},
			// containerboot writes its socket, in-memory state dir, and TLS
			// certs under /tmp; a dedicated emptyDir (added to the pod
			// alongside this sidecar) keeps the rootfs read-only without
			// exposing tailscaled's LocalAPI socket to the hermes container.
			{
				Name:      tailscaleTmpVolume,
				MountPath: "/tmp",
			},
		},
		// RunAsNonRoot/RunAsUser are set here defensively: the pod-level
		// security context is replaced wholesale when users set
		// spec.security.podSecurityContext.
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             Ptr(true),
			RunAsUser:                Ptr(int64(1000)),
			ReadOnlyRootFilesystem:   Ptr(true),
			AllowPrivilegeEscalation: Ptr(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}
}
