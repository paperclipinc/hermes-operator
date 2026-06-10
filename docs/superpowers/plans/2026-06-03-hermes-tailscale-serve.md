# Tailscale Serve Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a first-class `spec.tailscale.mode=serve` field to `HermesInstance` that exposes the hermes gateway (container port 8443) over a Tailscale tailnet via Tailscale Serve, using an ephemeral node with a stable hostname.

**Architecture:** A `tailscale/tailscale` sidecar is injected into the existing hermes StatefulSet pod (userspace networking; containerboot's default in-memory state, ephemeral via the reusable + ephemeral auth key). Serve config is rendered into the per-instance ConfigMap and mounted read-only, mapping tailnet `:443` to `http://127.0.0.1:8443`. The default-deny NetworkPolicy gains Tailscale UDP egress; a `TailscaleReady` condition and webhook validation follow the existing gateway patterns.

**Tech Stack:** Go 1.24, kubebuilder v4 / controller-runtime, Ginkgo/Gomega (e2e), testify (unit), Helm.

**Reference spec:** `docs/superpowers/specs/2026-06-03-hermes-tailscale-serve-design.md`

**Conventions (from `CLAUDE.md`):** No em or en dashes anywhere (code, comments, commits, docs). Use ASCII `-` for compounds/ranges and `.,;:` as connectors.

---

## File Structure

| File | Responsibility | Change |
|------|----------------|--------|
| `api/v1/hermesinstance_types.go` | `TailscaleSpec`, `TailscaleAuthKey`, `TailscaleImageSpec`, `Tailscale` field, `ConditionTailscaleReady` | Modify |
| `api/v1/zz_generated.deepcopy.go` | deepcopy methods | Regenerated |
| `config/crd/bases/*.yaml`, `charts/hermes-operator/templates/crds/*.yaml`, `docs/api-reference.md` | CRD + docs | Regenerated |
| `internal/resources/tailscale.go` | `tailscaleEnabled`, `BuildTailscaleSidecar`, `BuildTailscaleServeConfig`, volume helpers, defaults | Create |
| `internal/resources/tailscale_test.go` | unit tests for the above | Create |
| `internal/resources/configmap.go` | add serve-config key when enabled | Modify |
| `internal/resources/statefulset.go` | inject sidecar + serve-config volume | Modify |
| `internal/resources/networkpolicy.go` | `buildTailscaleEgressRules` wired into `buildEgressRules` | Modify |
| `internal/controller/hermesinstance_controller.go` | `TailscaleReady` reconcile step | Modify |
| `internal/webhook/webhook_hermesinstance_validate.go` | `validateTailscale` | Modify |
| `test/e2e/tailscale_test.go`, `test/e2e/testdata/hermesinstance-tailscale.yaml` | e2e coverage | Create |
| `README.md`, `examples/full-featured/*`, `ROADMAP.md` | docs reintroduce the shipped field | Modify |

---

## Task 1: CRD types for `spec.tailscale`

**Files:**
- Modify: `api/v1/hermesinstance_types.go`

- [ ] **Step 1: Add the condition constant**

In the condition constants block (near `ConditionMigrationCompleted`, around `api/v1/hermesinstance_types.go:1001-1025`), add:

```go
	// ConditionTailscaleReady reports whether the operator-managed Tailscale
	// sidecar wiring is up to date.
	ConditionTailscaleReady = "TailscaleReady"
```

- [ ] **Step 2: Add the spec types**

Append near the other feature sub-specs (e.g. after `HonchoSpec`):

```go
// TailscaleSpec configures exposing the hermes gateway over a Tailscale tailnet.
type TailscaleSpec struct {
	// Enabled turns on the operator-managed Tailscale sidecar.
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Mode selects how the gateway is exposed over the tailnet. Only "serve"
	// is implemented today (private tailnet exposure with a Tailscale TLS cert).
	// +kubebuilder:validation:Enum=serve
	// +kubebuilder:default=serve
	// +optional
	Mode string `json:"mode,omitempty"`

	// AuthKey references the Secret holding a reusable, ephemeral Tailscale auth
	// key, exposed to the sidecar as TS_AUTHKEY. Required when Enabled is true.
	// +optional
	AuthKey *TailscaleAuthKey `json:"authKey,omitempty"`

	// Hostname overrides the tailnet/MagicDNS hostname. Defaults to metadata.name.
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// Image overrides the tailscale sidecar image.
	// +optional
	Image TailscaleImageSpec `json:"image,omitempty"`

	// Resources sets the sidecar resource requirements.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// TailscaleAuthKey points at the Secret key holding the Tailscale auth key.
type TailscaleAuthKey struct {
	// +optional
	SecretRef *corev1.SecretKeySelector `json:"secretRef,omitempty"`
}

// TailscaleImageSpec pins the tailscale sidecar image.
type TailscaleImageSpec struct {
	// +kubebuilder:default="tailscale/tailscale"
	// +optional
	Repository string `json:"repository,omitempty"`
	// +kubebuilder:default="v1.86.2"
	// +optional
	Tag string `json:"tag,omitempty"`
	// +kubebuilder:default="IfNotPresent"
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}
```

- [ ] **Step 3: Add the field to `HermesInstanceSpec`**

In `HermesInstanceSpec` (around `api/v1/hermesinstance_types.go:33-159`), next to `ProfileStore`, add:

```go
	// Tailscale exposes the gateway over a Tailscale tailnet.
	// +optional
	Tailscale TailscaleSpec `json:"tailscale,omitempty"`
```

- [ ] **Step 4: Regenerate deepcopy and manifests**

Run: `make generate manifests`
Expected: `api/v1/zz_generated.deepcopy.go` gains `DeepCopy*` for the new types; `config/crd/bases/*hermesinstances*.yaml` gains the `tailscale` schema. No errors.

- [ ] **Step 5: Sync Helm CRD template and api-docs**

Run: `make sync-chart-crds api-docs` (use the chart-CRD sync target this repo provides; check `Makefile` if the name differs) then `go build ./...`
Expected: `charts/hermes-operator/templates/crds/*` and `docs/api-reference.md` updated, build clean.

- [ ] **Step 6: Commit**

```bash
git add api/v1/ config/crd/ charts/hermes-operator/ docs/api-reference.md
git commit -m "feat(api): add spec.tailscale (serve mode) to HermesInstance"
```

---

## Task 2: Tailscale resource builders

**Files:**
- Create: `internal/resources/tailscale.go`
- Test: `internal/resources/tailscale_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/resources/tailscale_test.go`:

```go
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
	assert.False(t, tailscaleEnabled(minimalInstance()))
	assert.True(t, tailscaleEnabled(tailscaleInstance()))
}

func TestBuildTailscaleSidecar(t *testing.T) {
	t.Parallel()
	c := BuildTailscaleSidecar(tailscaleInstance())
	require.NotNil(t, c)
	assert.Equal(t, "tailscale", c.Name)
	assert.Equal(t, "tailscale/tailscale:v1.86.2", c.Image)

	env := map[string]corev1.EnvVar{}
	for _, e := range c.Env {
		env[e.Name] = e
	}
	require.Contains(t, env, "TS_AUTHKEY")
	require.NotNil(t, env["TS_AUTHKEY"].ValueFrom)
	assert.Equal(t, "hermes-tailscale", env["TS_AUTHKEY"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "authKey", env["TS_AUTHKEY"].ValueFrom.SecretKeyRef.Key)
	assert.Equal(t, "true", env["TS_USERSPACE"].Value)
	// containerboot defaults to --state=mem: --statedir=/tmp when neither
	// TS_KUBE_SECRET nor TS_STATE_DIR is set; do not override it.
	assert.NotContains(t, env, "TS_STATE_DIR")
	assert.NotContains(t, env, "TS_EXTRA_ARGS")
	// hostname defaults to metadata.name
	assert.Equal(t, tailscaleInstance().Name, env["TS_HOSTNAME"].Value)
	// serve config is referenced and mounted
	assert.Equal(t, "/etc/tailscale/serve.json", env["TS_SERVE_CONFIG"].Value)
	var sawMount bool
	for _, m := range c.VolumeMounts {
		if m.MountPath == "/etc/tailscale" {
			sawMount = true
		}
	}
	assert.True(t, sawMount)
}

func TestBuildTailscaleServeConfig(t *testing.T) {
	t.Parallel()
	raw := BuildTailscaleServeConfig(tailscaleInstance())
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &cfg))
	// Maps tailnet :443 to the local hermes gateway on 8443.
	assert.Contains(t, raw, "127.0.0.1:8443")
	assert.Contains(t, raw, "443")
}

func TestBuildTailscaleSidecar_HostnameOverride(t *testing.T) {
	t.Parallel()
	inst := tailscaleInstance()
	inst.Spec.Tailscale.Hostname = "custom-host"
	c := BuildTailscaleSidecar(inst)
	for _, e := range c.Env {
		if e.Name == "TS_HOSTNAME" {
			assert.Equal(t, "custom-host", e.Value)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/resources/ -run TestTailscale -run TestBuildTailscale -v`
Expected: compile failure / FAIL (`tailscaleEnabled`, `BuildTailscaleSidecar`, `BuildTailscaleServeConfig` undefined).

- [ ] **Step 3: Implement the builders**

Create `internal/resources/tailscale.go`:

```go
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
	// GatewayPort is the hermes gateway container port (statefulset.go).
	tailscaleLocalTarget = "http://127.0.0.1:8443"
)

func tailscaleEnabled(inst *hermesv1.HermesInstance) bool {
	return inst.Spec.Tailscale.Enabled != nil && *inst.Spec.Tailscale.Enabled
}

func tailscaleImage(inst *hermesv1.HermesInstance) string {
	img := inst.Spec.Tailscale.Image
	repo := img.Repository
	if repo == "" {
		repo = "tailscale/tailscale"
	}
	tag := img.Tag
	if tag == "" {
		tag = "v1.86.2"
	}
	return fmt.Sprintf("%s:%s", repo, tag)
}

func tailscaleHostname(inst *hermesv1.HermesInstance) string {
	if h := inst.Spec.Tailscale.Hostname; h != "" {
		return h
	}
	return inst.Name
}

// BuildTailscaleServeConfig renders the Tailscale Serve config JSON that fronts
// the local hermes gateway (127.0.0.1:8443) on tailnet :443.
func BuildTailscaleServeConfig(inst *hermesv1.HermesInstance) string {
	return fmt.Sprintf(`{
  "TCP": { "443": { "HTTPS": true } },
  "Web": {
    "${TS_CERT_DOMAIN}:443": {
      "Handlers": { "/": { "Proxy": "%s" } }
    }
  }
}`, tailscaleLocalTarget)
}

// BuildTailscaleSidecar builds the operator-managed tailscale sidecar container,
// or nil when tailscale is disabled.
func BuildTailscaleSidecar(inst *hermesv1.HermesInstance) *corev1.Container {
	if !tailscaleEnabled(inst) {
		return nil
	}
	ts := inst.Spec.Tailscale
	pullPolicy := ts.Image.ImagePullPolicy
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
			Name:      "TS_AUTHKEY",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: ts.AuthKey.SecretRef},
		})
	}

	return &corev1.Container{
		Name:            tailscaleContainerName,
		Image:           tailscaleImage(inst),
		ImagePullPolicy: pullPolicy,
		Env:             env,
		Resources:       ts.Resources,
		VolumeMounts: []corev1.VolumeMount{{
			Name:      tailscaleServeVolume,
			MountPath: tailscaleServeMount,
			ReadOnly:  true,
		}},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             boolPtr(false),
			ReadOnlyRootFilesystem:   boolPtr(false),
			AllowPrivilegeEscalation: boolPtr(false),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		},
	}
}

func boolPtr(b bool) *bool { return &b }
```

Note: if a pointer helper already exists in `internal/resources` (grep for `func boolPtr` / `Ptr[`), reuse it and delete the local `boolPtr` to keep DRY.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/resources/ -run 'TestTailscale|TestBuildTailscale' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resources/tailscale.go internal/resources/tailscale_test.go
git commit -m "feat(resources): tailscale sidecar and serve-config builders"
```

---

## Task 3: Render serve config into the ConfigMap

**Files:**
- Modify: `internal/resources/configmap.go`
- Test: `internal/resources/tailscale_test.go` (add a case)

- [ ] **Step 1: Write the failing test**

Add to `internal/resources/tailscale_test.go`:

```go
func TestBuildConfigMap_IncludesTailscaleServe(t *testing.T) {
	t.Parallel()
	cm := BuildConfigMap(tailscaleInstance())
	got, ok := cm.Data[tailscaleServeKey]
	require.True(t, ok, "ConfigMap must carry the tailscale serve config when enabled")
	assert.Contains(t, got, "127.0.0.1:8443")

	// Disabled instance must not carry the key.
	cmOff := BuildConfigMap(minimalInstance())
	_, ok = cmOff.Data[tailscaleServeKey]
	assert.False(t, ok)
}
```

If the ConfigMap builder is named differently or takes extra args, match the existing signature in `internal/resources/configmap.go` (grep for `func Build.*ConfigMap`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -run TestBuildConfigMap_IncludesTailscaleServe -v`
Expected: FAIL (key absent).

- [ ] **Step 3: Implement**

In `internal/resources/configmap.go`, where the `Data` map is populated, add before returning:

```go
	if tailscaleEnabled(inst) {
		cm.Data[tailscaleServeKey] = BuildTailscaleServeConfig(inst)
	}
```

Use the actual local variable name for the ConfigMap object in that function.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/resources/ -run TestBuildConfigMap_IncludesTailscaleServe -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resources/configmap.go internal/resources/tailscale_test.go
git commit -m "feat(resources): emit tailscale serve config in the instance ConfigMap"
```

---

## Task 4: Inject the sidecar and serve-config volume into the StatefulSet

**Files:**
- Modify: `internal/resources/statefulset.go`
- Test: `internal/resources/statefulset_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/resources/statefulset_test.go`:

```go
func TestBuildStatefulSet_TailscaleSidecar(t *testing.T) {
	t.Parallel()
	sts := BuildStatefulSet(tailscaleInstance(), nil)

	var ts *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "tailscale" {
			ts = &sts.Spec.Template.Spec.Containers[i]
		}
	}
	require.NotNil(t, ts, "tailscale sidecar must be present when enabled")
	assert.Equal(t, "tailscale/tailscale:v1.86.2", ts.Image)

	var sawVol bool
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == "tailscale-serve" {
			require.NotNil(t, v.ConfigMap)
			sawVol = true
		}
	}
	assert.True(t, sawVol, "serve-config volume must be mounted from the ConfigMap")

	// Disabled: no sidecar.
	stsOff := BuildStatefulSet(minimalInstance(), nil)
	for _, c := range stsOff.Spec.Template.Spec.Containers {
		assert.NotEqual(t, "tailscale", c.Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -run TestBuildStatefulSet_TailscaleSidecar -v`
Expected: FAIL (no tailscale container).

- [ ] **Step 3: Implement**

In `internal/resources/statefulset.go`, at the user-sidecar append point (around line 219, `podSpec.Containers = append(podSpec.Containers, inst.Spec.Sidecars...)`), insert just before it:

```go
	if c := BuildTailscaleSidecar(inst); c != nil {
		podSpec.Containers = append(podSpec.Containers, *c)
	}
```

At the volumes assembly (around lines 268-271, where ConfigMap/PVC/user volumes are appended), add:

```go
	if tailscaleEnabled(inst) {
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: "tailscale-serve",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(inst)},
					Items: []corev1.KeyToPath{{
						Key:  tailscaleServeKey,
						Path: tailscaleServeFile,
					}},
				},
			},
		})
	}
```

Use the repo's actual ConfigMap-name helper (grep `func ConfigMapName` or how the existing config volume references the ConfigMap) and the actual pod spec variable name.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/resources/ -run 'TestBuildStatefulSet' -v`
Expected: PASS (new test and existing sidecar tests).

- [ ] **Step 5: Commit**

```bash
git add internal/resources/statefulset.go internal/resources/statefulset_test.go
git commit -m "feat(resources): inject tailscale sidecar and serve-config volume"
```

---

## Task 5: NetworkPolicy egress for Tailscale

**Files:**
- Modify: `internal/resources/networkpolicy.go`
- Test: `internal/resources/networkpolicy_test.go` (or `tailscale_test.go` if simpler)

- [ ] **Step 1: Write the failing test**

Add (in `internal/resources/networkpolicy_test.go`, matching the existing test file's package and helpers):

```go
func TestBuildNetworkPolicy_TailscaleEgress(t *testing.T) {
	t.Parallel()
	np := BuildNetworkPolicy(tailscaleInstance())

	var sawUDP3478, sawUDP41641 bool
	for _, e := range np.Spec.Egress {
		for _, p := range e.Ports {
			if p.Protocol != nil && *p.Protocol == corev1.ProtocolUDP && p.Port != nil {
				switch p.Port.IntValue() {
				case 3478:
					sawUDP3478 = true
				case 41641:
					sawUDP41641 = true
				}
			}
		}
	}
	assert.True(t, sawUDP3478, "expected STUN UDP/3478 egress")
	assert.True(t, sawUDP41641, "expected Tailscale UDP/41641 egress")

	// Disabled: no UDP tailscale rules.
	npOff := BuildNetworkPolicy(minimalInstance())
	for _, e := range npOff.Spec.Egress {
		for _, p := range e.Ports {
			if p.Protocol != nil && *p.Protocol == corev1.ProtocolUDP && p.Port != nil {
				assert.NotEqual(t, 41641, p.Port.IntValue())
			}
		}
	}
}
```

If `BuildNetworkPolicy` returns a pointer or takes extra args, match the existing signature.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -run TestBuildNetworkPolicy_TailscaleEgress -v`
Expected: FAIL (no UDP rules).

- [ ] **Step 3: Implement**

In `internal/resources/networkpolicy.go`, add:

```go
func buildTailscaleEgressRules(inst *hermesv1.HermesInstance) []networkingv1.NetworkPolicyEgressRule {
	if !tailscaleEnabled(inst) {
		return nil
	}
	udp := corev1.ProtocolUDP
	stun := intstr.FromInt32(3478)
	direct := intstr.FromInt32(41641)
	return []networkingv1.NetworkPolicyEgressRule{{
		// Tailscale direct connections (STUN + WireGuard). Control plane and
		// DERP relay use TCP/443, already allowed by the baseline.
		Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: &udp, Port: &stun},
			{Protocol: &udp, Port: &direct},
		},
	}}
}
```

Then in `buildEgressRules()` (around lines 101-132), append the result:

```go
	rules = append(rules, buildTailscaleEgressRules(inst)...)
```

Use the existing slice variable name in `buildEgressRules`. Ensure `intstr` and `networkingv1` imports already exist (they do for the other rules).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/resources/ -run TestBuildNetworkPolicy_TailscaleEgress -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resources/networkpolicy.go internal/resources/networkpolicy_test.go
git commit -m "feat(resources): allow tailscale UDP egress in the NetworkPolicy"
```

---

## Task 6: `TailscaleReady` reconcile step

**Files:**
- Modify: `internal/controller/hermesinstance_controller.go`

- [ ] **Step 1: Add the reconcile step**

The StatefulSet and ConfigMap already carry the sidecar/serve config, so this step has no separate resource to create; it only reports readiness once those are reconciled. Add a step entry to the `steps` slice (around lines 119-147), after the StatefulSet step and before/after `ProfileStoreReady`, gated on enablement:

```go
	if tailscaleEnabledForInstance(inst) {
		steps = append(steps, reconcileStep{
			name: "Tailscale",
			cond: hermesv1.ConditionTailscaleReady,
			fn:   r.reconcileTailscale,
		})
	}
```

Match the actual struct/type used for the steps slice (the recon shows an anonymous struct with `name string`, `cond string`, `fn func(...) error`; if it is anonymous, append a literal of that anonymous type instead of a named `reconcileStep`).

- [ ] **Step 2: Implement the reconcile function and helper**

Add to the controller:

```go
// reconcileTailscale is a no-op resource step: the sidecar and serve config are
// owned by the StatefulSet and ConfigMap reconciled earlier. It exists so the
// TailscaleReady condition tracks the feature explicitly.
func (r *HermesInstanceReconciler) reconcileTailscale(ctx context.Context, inst *hermesv1.HermesInstance) error {
	return nil
}

func tailscaleEnabledForInstance(inst *hermesv1.HermesInstance) bool {
	return inst.Spec.Tailscale.Enabled != nil && *inst.Spec.Tailscale.Enabled
}
```

(The `resources.tailscaleEnabled` helper is unexported; this small controller-local mirror avoids exporting it. If the controller already imports `resources` and an exported predicate is preferred, export `TailscaleEnabled` in Task 2 instead and reuse it here.)

- [ ] **Step 3: Build and run controller tests**

Run: `go build ./... && go test ./internal/controller/ -run Tailscale -v`
Expected: build clean; if there is an envtest condition test, it passes. If no test targets match, run `go test ./internal/controller/ -count=1` and expect PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/controller/hermesinstance_controller.go
git commit -m "feat(controller): add TailscaleReady reconcile step"
```

---

## Task 7: Webhook validation

**Files:**
- Modify: `internal/webhook/webhook_hermesinstance_validate.go`
- Test: matching `*_test.go` in `internal/webhook/`

- [ ] **Step 1: Write the failing test**

Add to the webhook test file (match its package and the existing validator construction):

```go
func TestValidateTailscale_RequiresAuthKey(t *testing.T) {
	v := &HermesInstanceValidator{Client: fakeClientWithNoSecrets()}
	inst := tailscaleEnabledInstanceNoKey() // enabled=true, authKey=nil
	_, err := v.ValidateCreate(context.Background(), inst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.tailscale.authKey")
}

func TestValidateTailscale_MissingSecretWarns(t *testing.T) {
	v := &HermesInstanceValidator{Client: fakeClientWithNoSecrets()}
	inst := tailscaleEnabledInstanceWithKey() // enabled=true, authKey.secretRef set, secret absent
	warns, err := v.ValidateCreate(context.Background(), inst)
	require.NoError(t, err)
	assert.NotEmpty(t, warns)
}
```

Reuse the existing test helpers for building a fake client and instances (grep the webhook test file for how `validateGateways` is tested and copy that scaffolding; do not invent new helpers if equivalents exist).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/webhook/ -run TestValidateTailscale -v`
Expected: FAIL (`validateTailscale` not wired).

- [ ] **Step 3: Implement**

Add the method and wire it into `ValidateCreate` and `ValidateUpdate` (alongside the `validateGateways` call, around lines 35-58):

```go
func (v *HermesInstanceValidator) validateTailscale(ctx context.Context, inst *hermesv1.HermesInstance) (admission.Warnings, error) {
	ts := inst.Spec.Tailscale
	if ts.Enabled == nil || !*ts.Enabled {
		return nil, nil
	}
	if ts.AuthKey == nil || ts.AuthKey.SecretRef == nil {
		return nil, fmt.Errorf("spec.tailscale.authKey.secretRef is required when tailscale is enabled")
	}
	ref := ts.AuthKey.SecretRef
	var s corev1.Secret
	err := v.Client.Get(ctx, types.NamespacedName{Namespace: inst.Namespace, Name: ref.Name}, &s)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return admission.Warnings{fmt.Sprintf(
				"spec.tailscale.authKey.secretRef references Secret %q which is not present yet in namespace %q",
				ref.Name, inst.Namespace)}, nil
		}
		return nil, fmt.Errorf("look up spec.tailscale.authKey.secretRef: %w", err)
	}
	if ref.Key != "" {
		if _, ok := s.Data[ref.Key]; !ok {
			return admission.Warnings{fmt.Sprintf(
				"spec.tailscale.authKey.secretRef references key %q in Secret %q which is not present",
				ref.Key, ref.Name)}, nil
		}
	}
	return nil, nil
}
```

In `ValidateCreate` and `ValidateUpdate`, after the gateway validation block:

```go
	tsWarns, tsErr := v.validateTailscale(ctx, inst)
	warnings = append(warnings, tsWarns...)
	if tsErr != nil {
		return warnings, tsErr
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/webhook/ -run TestValidateTailscale -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/webhook/
git commit -m "feat(webhook): validate spec.tailscale auth key reference"
```

---

## Task 8: E2E coverage

**Files:**
- Create: `test/e2e/testdata/hermesinstance-tailscale.yaml`
- Create: `test/e2e/tailscale_test.go`

- [ ] **Step 1: Add the test manifest**

Create `test/e2e/testdata/hermesinstance-tailscale.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: e2e-tailscale-auth
  namespace: default
stringData:
  authKey: tskey-dummy-not-a-real-key
---
apiVersion: hermes.agent/v1
kind: HermesInstance
metadata:
  name: e2e-tailscale
  namespace: default
spec:
  image:
    repository: ghcr.io/paperclipinc/hermes-agent
    tag: "v2026.5.29.2"
  storage:
    size: 1Gi
  tailscale:
    enabled: true
    mode: serve
    authKey:
      secretRef:
        name: e2e-tailscale-auth
        key: authKey
```

- [ ] **Step 2: Write the e2e test**

Create `test/e2e/tailscale_test.go`, mirroring `test/e2e/gateways_honcho_test.go`:

```go
package e2e

import (
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HermesInstance with Tailscale serve on kind", Ordered, func() {
	BeforeAll(func() {
		if os.Getenv("HERMES_E2E_FULL") != "1" {
			Skip("set HERMES_E2E_FULL=1 to enable the full e2e suite")
		}
		out, err := kubectl("apply", "-f", "testdata/hermesinstance-tailscale.yaml")
		Expect(err).NotTo(HaveOccurred(), out)
	})

	AfterAll(func() {
		if os.Getenv("HERMES_E2E_FULL") != "1" {
			return
		}
		_, _ = kubectl("delete", "-f", "testdata/hermesinstance-tailscale.yaml", "--ignore-not-found=true")
	})

	It("injects the tailscale sidecar into the pod", func() {
		Eventually(func(g Gomega) {
			out, err := kubectl("get", "statefulset", "e2e-tailscale",
				"-n", "default",
				"-o", "jsonpath={.spec.template.spec.containers[*].name}")
			g.Expect(err).NotTo(HaveOccurred(), out)
			g.Expect(out).To(ContainSubstring("tailscale"))
		}).Should(Succeed())
	})

	It("emits a NetworkPolicy with UDP/41641 egress", func() {
		out, err := kubectl("get", "networkpolicy", "e2e-tailscale",
			"-n", "default",
			"-o", "jsonpath={.spec.egress[*].ports[*].port}")
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("41641"))
	})
})
```

(The dummy auth key never joins a real tailnet, so we assert resource shape, not connectivity. The StatefulSet may not reach Ready because tailscaled cannot authenticate; do not gate on `readyReplicas` here, unlike the gateways test.)

- [ ] **Step 3: Run unit + lint locally (e2e runs in CI)**

Run: `go build ./... && go vet ./... && go test ./internal/...`
Expected: build clean, all unit tests PASS. (E2E is gated behind `HERMES_E2E_FULL=1` and the kind job in CI.)

- [ ] **Step 4: Commit**

```bash
git add test/e2e/tailscale_test.go test/e2e/testdata/hermesinstance-tailscale.yaml
git commit -m "test(e2e): tailscale sidecar and NetworkPolicy egress"
```

---

## Task 9: Documentation

**Files:**
- Modify: `README.md`, `examples/full-featured/hermesinstance.yaml`, `examples/full-featured/README.md`, `ROADMAP.md`

- [ ] **Step 1: Reintroduce the field in the full-featured example**

In `examples/full-featured/hermesinstance.yaml`, replace the "Planned sidecars" comment (added by the #42 docs fix) with the real block:

```yaml
  tailscale:
    enabled: true
    mode: serve
    authKey:
      secretRef:
        name: hermes-tailscale
        key: authKey
```

(Leave `webTerminal` planned. Add a `kubectl create secret generic hermes-tailscale` line to the Prerequisites in `examples/full-featured/README.md`, and a `tailscale` row back into the fields table; remove the "Planned (not yet in the v1 CRD)" note's tailscale mention, keeping only webTerminal there.)

- [ ] **Step 2: Document in the main README**

Add a short "Tailscale Serve" subsection near the networking docs describing `spec.tailscale.{enabled,mode,authKey.secretRef,hostname}`, the ephemeral-node behavior, the reusable+ephemeral auth-key requirement, and the DERP fallback when UDP egress is blocked. No em or en dashes.

- [ ] **Step 3: Fix ROADMAP**

In `ROADMAP.md`, ensure tailscale is listed accurately under Shipped only as of this release (it was previously listed under v1.0.0 Shipped though never wired). Adjust the line so it reflects the version that actually ships this.

- [ ] **Step 4: Regenerate api-docs and verify clean tree**

Run: `make api-docs && git diff --stat`
Expected: only intended doc changes.

- [ ] **Step 5: Dash check (CLAUDE.md)**

Run: `grep -rnP '[\x{2013}\x{2014}]' README.md ROADMAP.md examples/full-featured/ docs/api-reference.md && echo FOUND || echo clean`
Expected: `clean`.

- [ ] **Step 6: Commit**

```bash
git add README.md ROADMAP.md examples/full-featured/ docs/api-reference.md
git commit -m "docs: document shipped spec.tailscale serve support"
```

---

## Task 10: Full verification before PR

- [ ] **Step 1: Generated artifacts in sync**

Run: `make generate manifests sync-chart-crds api-docs && git diff --exit-code`
Expected: exit 0 (no uncommitted drift). If drift, commit it.

- [ ] **Step 2: Full build, vet, lint, unit tests**

Run: `go build ./... && go vet ./... && make lint && go test ./internal/... ./api/...`
Expected: all PASS.

- [ ] **Step 3: Helm RBAC sync (if the repo enforces it)**

Run: the repo's Helm RBAC sync check (mirrors openclaw `hack/check-helm-rbac-sync.sh`; check `Makefile`/`.github/workflows` for the exact target). No new operator RBAC was added (ephemeral nodes need no Secret state), so expect PASS with no changes.

- [ ] **Step 4: Open the PR**

```bash
git push upstream feat/tailscale-serve
gh pr create --repo paperclipinc/hermes-operator --base main \
  --title "feat: spec.tailscale.mode=serve (private tailnet exposure)" \
  --body "Implements #42 per docs/superpowers/specs/2026-06-03-hermes-tailscale-serve-design.md."
```

---

## Self-Review Notes

- **Spec coverage:** API (Task 1) covers G1; serve config + sidecar + port 8443 (Tasks 2-4) cover G2; ephemeral node 2.1, sidecar model 2.2, serve config 2.3, NetworkPolicy 2.4, condition (Task 6), webhook (Task 7) cover G3; additive Service (no change) covers G4. Tests in Tasks 2,3,4,5,7,8 cover spec section 6. Docs (Task 9) cover section 7. NG1 enforced by the `Enum=serve` marker; NG2 by ephemeral state (no RBAC); NG3/NG4 untouched.
- **Type consistency:** `TailscaleSpec`, `TailscaleAuthKey`, `TailscaleImageSpec`, `tailscaleEnabled`, `BuildTailscaleSidecar`, `BuildTailscaleServeConfig`, `tailscaleServeKey`, `ConditionTailscaleReady` are used consistently across tasks.
- **Known adaptation points (verify against actual code, do not guess):** the ConfigMap builder name/signature (Task 3), the ConfigMap-name helper and pod-spec variable (Task 4), `BuildNetworkPolicy` signature and the egress slice variable (Task 5), the `steps` slice element type (Task 6), webhook test scaffolding and `admission.Warnings` import (Task 7), the Makefile chart-CRD-sync and lint target names (Tasks 1, 10). Each task says to match the existing pattern rather than assume.
