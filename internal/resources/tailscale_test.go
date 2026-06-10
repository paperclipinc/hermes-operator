package resources

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

func tailscaleInstance() *hermesv1.HermesInstance {
	inst := minimalInstance()
	enabled := true
	inst.Spec.Tailscale = hermesv1.TailscaleSpec{
		Enabled: &enabled,
		Mode:    "serve",
		AuthKey: &hermesv1.TailscaleAuthKey{
			SecretRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "hermes-tailscale"},
				Key:                  "authKey",
			},
		},
	}
	return inst
}

func TestTailscaleEnabled(t *testing.T) {
	t.Parallel()
	assert.False(t, tailscaleEnabled(minimalInstance()), "absent block means disabled")
	assert.True(t, tailscaleEnabled(tailscaleInstance()))
}

func envByName(c *corev1.Container, name string) *corev1.EnvVar {
	for i := range c.Env {
		if c.Env[i].Name == name {
			return &c.Env[i]
		}
	}
	return nil
}

func TestBuildTailscaleSidecar(t *testing.T) {
	t.Parallel()
	c := BuildTailscaleSidecar(tailscaleInstance())
	require.NotNil(t, c)

	assert.Equal(t, "tailscale", c.Name)
	assert.Equal(t, "tailscale/tailscale:v1.86.2", c.Image)
	assert.Equal(t, corev1.PullIfNotPresent, c.ImagePullPolicy, "defaults to IfNotPresent")

	authKey := envByName(c, "TS_AUTHKEY")
	require.NotNil(t, authKey, "TS_AUTHKEY env present")
	require.NotNil(t, authKey.ValueFrom)
	require.NotNil(t, authKey.ValueFrom.SecretKeyRef)
	assert.Equal(t, "hermes-tailscale", authKey.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "authKey", authKey.ValueFrom.SecretKeyRef.Key)

	userspace := envByName(c, "TS_USERSPACE")
	require.NotNil(t, userspace)
	assert.Equal(t, "true", userspace.Value)

	assert.Nil(t, envByName(c, "TS_STATE_DIR"), "containerboot default --state=mem: must not be overridden")
	assert.Nil(t, envByName(c, "TS_EXTRA_ARGS"), "containerboot default --state=mem: must not be overridden")

	hostname := envByName(c, "TS_HOSTNAME")
	require.NotNil(t, hostname)
	assert.Equal(t, "demo", hostname.Value, "defaults to metadata.name")

	serveConfig := envByName(c, "TS_SERVE_CONFIG")
	require.NotNil(t, serveConfig)
	assert.Equal(t, "/etc/tailscale/serve.json", serveConfig.Value)

	mounts := map[string]corev1.VolumeMount{}
	for _, m := range c.VolumeMounts {
		mounts[m.MountPath] = m
	}
	serve, ok := mounts["/etc/tailscale"]
	require.True(t, ok, "serve config volume mounted at /etc/tailscale")
	assert.Equal(t, "tailscale-serve", serve.Name)
	assert.True(t, serve.ReadOnly, "serve config mount is read-only")
	tmp, ok := mounts["/tmp"]
	require.True(t, ok, "writable /tmp mounted for containerboot state, socket, and certs")
	assert.Equal(t, "tailscale-tmp", tmp.Name)
	assert.False(t, tmp.ReadOnly, "/tmp mount must be writable")

	sc := c.SecurityContext
	require.NotNil(t, sc)
	require.NotNil(t, sc.RunAsNonRoot)
	assert.True(t, *sc.RunAsNonRoot)
	require.NotNil(t, sc.RunAsUser)
	assert.Equal(t, int64(1000), *sc.RunAsUser)
	require.NotNil(t, sc.ReadOnlyRootFilesystem)
	assert.True(t, *sc.ReadOnlyRootFilesystem)
	require.NotNil(t, sc.AllowPrivilegeEscalation)
	assert.False(t, *sc.AllowPrivilegeEscalation)
	require.NotNil(t, sc.Capabilities)
	assert.Equal(t, []corev1.Capability{"ALL"}, sc.Capabilities.Drop)
}

func TestBuildTailscaleSidecar_ImageOverride(t *testing.T) {
	t.Parallel()
	inst := tailscaleInstance()
	inst.Spec.Tailscale.Image = hermesv1.TailscaleImageSpec{
		Repository: "r/ts",
		Tag:        "v9",
		PullPolicy: "Always",
	}
	c := BuildTailscaleSidecar(inst)
	require.NotNil(t, c)

	assert.Equal(t, "r/ts:v9", c.Image)
	assert.Equal(t, corev1.PullAlways, c.ImagePullPolicy)
}

func TestBuildTailscaleSidecar_Disabled(t *testing.T) {
	t.Parallel()
	assert.Nil(t, BuildTailscaleSidecar(minimalInstance()))
}

func TestBuildTailscaleServeConfig(t *testing.T) {
	t.Parallel()
	cfg := BuildTailscaleServeConfig(tailscaleInstance())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(cfg), &parsed), "serve config must be valid JSON")
	assert.Contains(t, cfg, "127.0.0.1:8443")
	assert.Contains(t, cfg, "443")
	assert.Contains(t, cfg, "${TS_CERT_DOMAIN}")
}

// TestBuildTailscaleServeConfig_ModeAgnostic pins the assumption that the
// serve mapping is emitted regardless of Mode. The CRD enum admits only
// "serve", but Mode can be "" when the spec is built in Go (CRD defaulting
// only materializes when the tailscale key is present in the manifest).
// If this test breaks because a second mode was added, BuildTailscaleServeConfig
// must learn to branch on Mode.
func TestBuildTailscaleServeConfig_ModeAgnostic(t *testing.T) {
	t.Parallel()
	want := BuildTailscaleServeConfig(tailscaleInstance())

	for _, mode := range []string{"", "serve"} {
		inst := tailscaleInstance()
		inst.Spec.Tailscale.Mode = mode
		assert.Equal(t, want, BuildTailscaleServeConfig(inst),
			"serve config must be the serve mapping for Mode=%q", mode)
	}
}

func TestBuildConfigMap_IncludesTailscaleServe(t *testing.T) {
	t.Parallel()
	cm := BuildConfigMap(tailscaleInstance(), "")
	got, ok := cm.Data[tailscaleServeKey]
	require.True(t, ok, "ConfigMap must carry the tailscale serve config when enabled")
	assert.Contains(t, got, "127.0.0.1:8443")

	cmOff := BuildConfigMap(minimalInstance(), "")
	_, ok = cmOff.Data[tailscaleServeKey]
	assert.False(t, ok)
}

func TestBuildTailscaleSidecar_HostnameOverride(t *testing.T) {
	t.Parallel()
	inst := tailscaleInstance()
	inst.Spec.Tailscale.Hostname = "custom-host"
	c := BuildTailscaleSidecar(inst)
	require.NotNil(t, c)

	hostname := envByName(c, "TS_HOSTNAME")
	require.NotNil(t, hostname)
	assert.Equal(t, "custom-host", hostname.Value)
}
