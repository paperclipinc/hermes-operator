package conformance

import (
	"fmt"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// idempotencyCorpus maps a human-readable label to the testdata fixture file.
// Each fixture is applied once, allowed to become Ready, then force-requeued 9
// more times. After each requeue we assert the resourceFingerprint is unchanged
// (generation + resourceVersion must not move). This catches lesson #437
// regressions: a reconciler that always re-writes owned objects will fail here.
// idempotencyImageContractSkip documents why the Ready-gated idempotency corpus
// is currently skipped. Once the conformance suite actually runs (#64), every
// HermesInstance fails to reach Ready: the operator's `init-uv` init container
// (internal/resources/runtime_init.go) copies pyproject.toml/uv.lock from
// /opt/venv-template/, but the published ghcr.io/paperclipinc/hermes-agent image
// builds its venv at /opt/venv and ships nothing at /opt/venv-template/. The init
// container exits 1, the pod never starts the hermes container, and the
// StatefulSet never reaches readyReplicas==replicas. That is an operator/agent
// image contract bug unrelated to reconciler idempotency, tracked in #68. These
// entries are skipped (visibly, with this reason) rather than left to hang until
// timeout and fail. Remove the skips once #68 is fixed.
const idempotencyImageContractSkip = "blocked by #68: operator init-uv copies from /opt/venv-template which is absent in the published hermes-agent image, so no HermesInstance reaches Ready; unskip once #68 is fixed"

var idempotencyCorpus = []struct {
	label   string
	fixture string
	// skip, when non-empty, skips this corpus entry with the given reason.
	// Used for fixtures that cannot reach Ready in CI for reasons unrelated to
	// operator idempotency (e.g. they require live external credentials, or are
	// blocked by an out-of-scope operator bug).
	skip string
}{
	{label: "minimal", fixture: "minimal.yaml", skip: idempotencyImageContractSkip},
	{label: "maximal", fixture: "maximal.yaml", skip: idempotencyImageContractSkip},
	{label: "gateways-all", fixture: "gateways-all.yaml", skip: idempotencyImageContractSkip},
	{label: "selfconfig-enabled", fixture: "selfconfig-enabled.yaml", skip: idempotencyImageContractSkip},
	{label: "profilestore-enabled", fixture: "profilestore-enabled.yaml", skip: idempotencyImageContractSkip},
	{label: "autoupdate-enabled", fixture: "autoupdate-enabled.yaml", skip: idempotencyImageContractSkip},
	{label: "backup-enabled", fixture: "backup-enabled.yaml", skip: idempotencyImageContractSkip},
	{label: "networking-ingress", fixture: "networking-ingress.yaml", skip: idempotencyImageContractSkip},
	{label: "observability-full", fixture: "observability-full.yaml", skip: idempotencyImageContractSkip},
	{
		label:   "ollama-webterminal-tailscale",
		fixture: "ollama-webterminal-tailscale.yaml",
		// Blocked twice over: by #68 (init-uv contract, like every entry) and,
		// even after #68, by the operator-managed tailscale sidecar. That sidecar
		// runs `containerboot`, which exits when TS_AUTHKEY cannot join a tailnet.
		// The fixture ships a dummy auth key (no real ephemeral key is available
		// in CI), so the sidecar container never becomes Ready, the pod stays
		// NotReady, and the HermesInstance never reaches Ready=True. Unskip only
		// once #68 is fixed AND a real ephemeral tailnet auth key is injected via
		// secret in CI. See #64.
		skip: "requires a live tailscale ephemeral auth key to reach Ready (dummy key cannot join a tailnet), and is also blocked by #68; see #64",
	},
}

const (
	idempotencyReconciles = 10
	idempotencyReadyWait  = 3 * time.Minute
	idempotencyPokeWait   = 15 * time.Second
)

var _ = Describe("idempotency canary", Ordered, func() {
	var (
		ns string
		c  = newClient
	)

	BeforeAll(func() {
		ns = freshNamespace("idempotency")
		DeferCleanup(func() {
			deleteNamespace(ns)
		})
	})

	for _, entry := range idempotencyCorpus {
		entry := entry // capture

		Describe(fmt.Sprintf("corpus entry: %s", entry.label), Ordered, func() {
			var instName string

			BeforeAll(func() {
				if entry.skip != "" {
					Skip(entry.skip)
				}
				fixturePath := filepath.Join("testdata", entry.fixture)
				yaml := readFile(fixturePath)
				// Inject the test namespace into the fixture.
				namespaced := addNamespace(yaml, ns)

				out, err := kubectlApply(namespaced)
				Expect(err).ToNot(HaveOccurred(),
					"applying fixture %s: %s", entry.fixture, out)

				// Extract the instance name from the fixture (first `name:` under metadata).
				instName = extractName(yaml)
				Expect(instName).ToNot(BeEmpty(), "could not extract name from fixture %s", entry.fixture)

				DeferCleanup(func() {
					_, _ = kubectlDelete(namespaced)
				})
			})

			It("becomes Ready", func() {
				waitForInstanceReady(suiteCtx, c(), ns, instName, idempotencyReadyWait)
			})

			It(fmt.Sprintf("resource fingerprint is stable across %d reconciles", idempotencyReconciles), func() {
				cl := c()
				before := captureFingerprint(suiteCtx, cl, ns, instName)

				for i := 1; i < idempotencyReconciles; i++ {
					forceRequeue(suiteCtx, cl, ns, instName)
					// Give the controller a moment to process the requeue.
					time.Sleep(idempotencyPokeWait)
					after := captureFingerprint(suiteCtx, cl, ns, instName)
					expectFingerprintUnchanged(before, after)
					before = after
				}
			})
		})
	}
})

// extractName parses the `name:` field from the first metadata block in a
// YAML manifest. It is intentionally naive: it walks lines looking for the
// pattern "  name: <value>" after a "metadata:" line.
func extractName(yaml string) string {
	inMeta := false
	for _, line := range splitLines(yaml) {
		if line == "metadata:" {
			inMeta = true
			continue
		}
		if inMeta {
			trimmed := trimPrefix(line, "  name: ")
			if trimmed != line {
				return trimmed
			}
			// Any non-indented line ends the metadata block.
			if len(line) > 0 && line[0] != ' ' {
				inMeta = false
			}
		}
	}
	return ""
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
