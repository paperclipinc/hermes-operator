# hermes-operator: `spec.tailscale.mode=serve` Design

- **Status:** Proposed (2026-06-03)
- **Owner:** stubbi (jannes@aqora.io)
- **Tracking issue:** [#42](https://github.com/paperclipinc/hermes-operator/issues/42)
- **Repo:** `paperclipinc/hermes-operator`
- **Manages:** [nousresearch/hermes-agent](https://github.com/nousresearch/hermes-agent)

## 1. Context & Goals

The full-featured example and `ROADMAP.md` advertise a first-class `spec.tailscale`
block, but the v1 CRD never exposed it. Tailscale today only works through the
generic `spec.sidecars[]` escape hatch (see
`test/conformance/testdata/ollama-webterminal-tailscale.yaml`), where the user
hand-writes a `tailscale/tailscale` container and wires `TS_AUTHKEY` themselves.
The misleading docs were corrected separately (PR for #42); this spec covers the
actual feature.

Homelab and private-network operators want LAN-only reachability over a tailnet
without standing up MetalLB + LoadBalancer + cert-manager + Ingress. A first-class
`spec.tailscale.mode=serve` binding makes the hermes gateway reachable over the
tailnet with a per-instance MagicDNS name and a Tailscale-issued TLS cert, with no
public ingress.

### Goals

- **G1:** Declarative `spec.tailscale.{enabled,mode,authKey.secretRef,hostname?,image?,resources?}` on the v1 CRD.
- **G2:** `mode=serve` exposes the hermes gateway (container port `8443`, name `gateway`, see `internal/resources/statefulset.go:62-64`) over the tailnet via Tailscale Serve, terminating TLS with the tailnet cert.
- **G3:** Secure by default and consistent with existing features: ephemeral node identity, no new cluster-wide RBAC, default-deny NetworkPolicy extended with only the egress Tailscale needs, `TailscaleReady` status condition, webhook validation mirroring gateways.
- **G4:** Additive. Enabling Tailscale does not change the existing `spec.networking.service`; operators can drop the LoadBalancer themselves once they rely on Serve.

### Non-goals

- **NG1: Funnel (public internet exposure).** `mode` enum is left extensible, but only `serve` ships now. (Confirmed scope decision, #42.)
- **NG2: Persisted node identity (Secret/PVC state).** We use ephemeral nodes (decision below). No `TS_KUBE_SECRET` RBAC, no per-instance state PVC.
- **NG3: Subnet routing / exit-node advertising.** Single-pod Serve only.
- **NG4: `spec.webTerminal` / `ollama`.** Separate Plan 3 sidecars, out of scope here.

## 2. Key design decisions

### 2.1 Node identity: ephemeral + fixed hostname

tailscaled needs identity state across restarts. We use an **ephemeral auth key**
(`TS_AUTHKEY` from the user's Secret, expected to be reusable + ephemeral) plus a
**fixed hostname** (`TS_HOSTNAME`, defaulting to the instance name, overridable via
`spec.tailscale.hostname`).

- The node auto-removes from the tailnet when the pod terminates (no stale node
  cleanup, no orphaned MagicDNS entries).
- On restart it re-registers under the same hostname, so the MagicDNS name is
  stable from the operator's and user's point of view.
- No PVC and no `TS_KUBE_SECRET`, so no extra per-instance RBAC and no new
  ServiceAccount wiring. This keeps the blast radius identical to the existing
  gateway/honcho features.

Rejected alternatives: `TS_KUBE_SECRET` state (adds Secret read/write RBAC to the
per-instance Role in `internal/resources/rbac.go`), and a state PVC (adds storage
and cleanup per instance). Both buy "same node object across restarts", which we
do not need because hostname stability already gives stable MagicDNS.

### 2.2 Sidecar in the hermes pod (not a companion Deployment)

Serve proxies tailnet traffic to a local listener, so the tailscale container must
share a network namespace with hermes and reach `localhost:8443`. It is therefore
a **sidecar in the same StatefulSet pod**, following the user-sidecar wiring point
at `internal/resources/statefulset.go:219` (`append(podSpec.Containers, ...)`),
**not** the companion-Deployment model used by Honcho
(`internal/resources/honcho.go`). The operator-built tailscale container is
prepended to (or merged ahead of) the user-supplied `spec.sidecars`.

### 2.3 Serve configuration

The sidecar runs the official `tailscale/tailscale` image in userspace-networking
mode with:

- `TS_AUTHKEY` from `spec.tailscale.authKey.secretRef`
- `TS_HOSTNAME` = `spec.tailscale.hostname` or `metadata.name`
- `TS_USERSPACE=true`; neither `TS_KUBE_SECRET` nor `TS_STATE_DIR` is set, so
  containerboot defaults to `--state=mem: --statedir=/tmp` (in-memory, ephemeral
  state). Ephemerality itself comes from the user-supplied reusable + ephemeral
  auth key.
- `TS_SERVE_CONFIG` pointing at a small serve config JSON mounted from the
  instance ConfigMap, mapping tailnet `:443` to `http://127.0.0.1:8443`.

The sidecar runs as UID 1000 (`runAsNonRoot`) with a read-only root filesystem;
a dedicated `/tmp` emptyDir gives containerboot a writable spot for its state
dir, LocalAPI socket, and TLS certs.

The serve-config JSON is rendered by a new builder and added to the existing
per-instance ConfigMap volume, mounted read-only into the sidecar. This keeps Serve
declarative and reconcilable rather than relying on an exec hook.

### 2.4 NetworkPolicy egress

The default-deny baseline (`internal/resources/networkpolicy.go:104-120`) already
allows DNS and TCP/443 to any peer, which covers Tailscale control plane and
DERP-relayed traffic. Direct (non-relayed) connections additionally use UDP. We add
a dedicated `buildTailscaleEgressRules()` appended in `buildEgressRules()` that,
when `tailscale.enabled`, allows:

- UDP/3478 (STUN) to any peer
- UDP/41641 (default Tailscale direct port) to any peer

If those are blocked by a stricter CNI, Tailscale transparently falls back to
DERP-over-443, which the baseline already permits, so the feature degrades to
relayed connectivity rather than failing. This is documented.

## 3. API surface

New type in `api/v1/hermesinstance_types.go`, added as `Tailscale TailscaleSpec`
on `HermesInstanceSpec`, mirroring the `HonchoSpec`/gateway secret-ref pattern:

```go
type TailscaleSpec struct {
    // +kubebuilder:default=false
    // +optional
    Enabled *bool `json:"enabled,omitempty"`

    // Mode selects how the gateway is exposed over the tailnet.
    // Only "serve" is implemented today.
    // +kubebuilder:validation:Enum=serve
    // +kubebuilder:default=serve
    // +optional
    Mode string `json:"mode,omitempty"`

    // AuthKey references the Secret holding a reusable, ephemeral Tailscale
    // auth key (exposed to the sidecar as TS_AUTHKEY). Required when enabled.
    // +optional
    AuthKey *TailscaleAuthKey `json:"authKey,omitempty"`

    // Hostname overrides the tailnet/MagicDNS hostname. Defaults to metadata.name.
    // +optional
    Hostname string `json:"hostname,omitempty"`

    // +optional
    Image TailscaleImageSpec `json:"image,omitempty"`
    // +optional
    Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

type TailscaleAuthKey struct {
    // +optional
    SecretRef *corev1.SecretKeySelector `json:"secretRef,omitempty"`
}
```

`authKey.secretRef` is modelled as a nested struct (matching the example's
`authKey.secretRef.{name,key}` shape) rather than a bare `SecretKeySelector`, so the
documented YAML keeps working unchanged.

## 4. Components & data flow

1. **CRD + deepcopy + manifests.** Add `TailscaleSpec`, run `make generate manifests`, sync the Helm CRD template, regenerate `docs/api-reference.md`.
2. **Resource builders** (`internal/resources/`):
   - `tailscale.go`: `tailscaleEnabled(inst)`, `BuildTailscaleSidecar(inst) *corev1.Container`, `BuildTailscaleServeConfig(inst) string` (JSON), and a volume/mount helper.
   - `statefulset.go`: inject the sidecar at the existing sidecar append point; add the serve-config to the ConfigMap volume.
   - `configmap.go`: include the serve-config JSON key when enabled.
   - `networkpolicy.go`: `buildTailscaleEgressRules()` wired into `buildEgressRules()`.
3. **Controller** (`internal/controller/hermesinstance_controller.go`): add a `TailscaleReady` step to the reconcile `steps` slice and the `ConditionTailscaleReady` constant; no new reconcile resource beyond what the StatefulSet/ConfigMap already own.
4. **Webhook** (`internal/webhook/webhook_hermesinstance_validate.go`): `validateTailscale()` requiring `authKey.secretRef` when enabled and cross-checking the Secret/key (warning when absent, mirroring `validateGateways`).

Data flow: user creates Secret + `HermesInstance{spec.tailscale.enabled}` -> webhook validates -> reconciler renders ConfigMap (serve config) + StatefulSet (sidecar) + NetworkPolicy (egress) -> tailscaled joins tailnet as `<hostname>`, serves `:443 -> 127.0.0.1:8443` -> reachable at `https://<hostname>.<tailnet>.ts.net`.

## 5. Error handling & status

- Webhook rejects `enabled && authKey.secretRef == nil`; warns (does not block) when the Secret or key is missing yet, matching gateways.
- `TailscaleReady` is set False/`Error` if rendering fails, True/`Reconciled` once the StatefulSet carries the sidecar. Pod-level tailscaled health is observed via the StatefulSet readiness already gating `Ready`; we do not add a bespoke probe in v1.
- Degraded connectivity (UDP blocked) falls back to DERP and is documented, not surfaced as a failure.

## 6. Testing

- **Unit** (`internal/resources/statefulset_test.go`, new `tailscale_test.go`): assert the sidecar is present with correct image, `TS_AUTHKEY` env from the secret ref, `TS_HOSTNAME`, no state-dir override (neither `TS_STATE_DIR` nor `TS_EXTRA_ARGS` is set, so containerboot keeps its in-memory default), the hardened security context, and the serve-config plus `/tmp` mounts; assert the serve config JSON maps `:443 -> 127.0.0.1:8443`; assert NetworkPolicy gains the UDP egress rules only when enabled.
- **Webhook** unit tests: required-secret rejection + missing-secret warning.
- **E2E** (`test/e2e/tailscale_test.go`, gated by `HERMES_E2E_FULL=1`): apply an instance with tailscale enabled and a dummy auth-key Secret, assert the StatefulSet reaches `readyReplicas=1` with a `tailscale` container, and assert the NetworkPolicy carries the expected egress. (Real tailnet join is not exercised in CI; covered by manual/homelab validation per the issue author's offer to test.)
- **Conformance:** replace the raw-sidecar `ollama-webterminal-tailscale.yaml` usage with the first-class field for the tailscale portion.

## 7. Docs & rollout

- README + full-featured example: reintroduce `spec.tailscale` as a real, shipped field (reverting the "Planned" note added for #42 once this lands).
- Correct `ROADMAP.md`, which currently lists tailscale under "Shipped (v1.0.0)" though it was never wired; move it accurately once shipped.
- `make api-docs` regenerates `docs/api-reference.md`.
- Ships as a `feat:` so release-please cuts a minor version.

## 8. Open questions

- Exact `tailscale/tailscale` image tag to pin (track upstream stable). Defaulted in `TailscaleImageSpec`, overridable.
- Whether to default `spec.networking.service.type` down to `ClusterIP` when tailscale is the only intended exposure. Deferred: additive in v1, revisit if users ask.
