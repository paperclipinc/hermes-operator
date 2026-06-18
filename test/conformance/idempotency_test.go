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
// The Ready-gated corpus stays skipped only until the agent image is republished.
//
// The runtime blockers that previously made Ready unreachable are FIXED in the
// operator code (PR #90, issue #89): the operator now runs the upstream s6
// hermes-agent image with `gateway run` + the OpenAI API server, probes
// HTTPGet /health, and injects a placeholder LLM provider so the gateway comes
// up without live calls (validated end-to-end on kind — an instance reaches
// Ready=True). The old failure modes are gone: no one-shot `hermes-agent run`,
// no TCPSocket :8443 with nothing listening, no LLM-credential requirement just
// to start, no missing modules (the upstream image ships everything incl. a
// browser).
//
// The one thing the conformance suite still needs is the *published* image to BE
// that upstream-based runtime: these fixtures pin
// `ghcr.io/paperclipinc/hermes-agent:v2026.5.29.2`, which only ships the new
// runtime after the FROM-upstream Dockerfile (PR #90) is merged and that tag is
// republished. Un-skip in the follow-up once the image is republished.
// (waitForInstanceReady dumps pod diagnostics on timeout if anything regresses.)
const idempotencyReadyBlockedSkip = "runtime fixed in #90 (upstream s6 image + gateway run + /health, validated on kind); un-skip once ghcr.io/paperclipinc/hermes-agent:v2026.5.29.2 is republished FROM upstream"

var idempotencyCorpus = []struct {
	label   string
	fixture string
	// skip, when non-empty, skips this corpus entry with the given reason.
	// Used for fixtures that cannot reach Ready in CI for reasons unrelated to
	// operator idempotency (e.g. they require live external credentials, or are
	// blocked by an out-of-scope operator bug).
	skip string
}{
	{label: "minimal", fixture: "minimal.yaml", skip: idempotencyReadyBlockedSkip},
	{label: "maximal", fixture: "maximal.yaml", skip: idempotencyReadyBlockedSkip},
	{label: "gateways-all", fixture: "gateways-all.yaml", skip: idempotencyReadyBlockedSkip},
	{label: "selfconfig-enabled", fixture: "selfconfig-enabled.yaml", skip: idempotencyReadyBlockedSkip},
	{label: "profilestore-enabled", fixture: "profilestore-enabled.yaml", skip: idempotencyReadyBlockedSkip},
	{label: "autoupdate-enabled", fixture: "autoupdate-enabled.yaml", skip: idempotencyReadyBlockedSkip},
	{label: "backup-enabled", fixture: "backup-enabled.yaml", skip: idempotencyReadyBlockedSkip},
	{label: "networking-ingress", fixture: "networking-ingress.yaml", skip: idempotencyReadyBlockedSkip},
	{label: "observability-full", fixture: "observability-full.yaml", skip: idempotencyReadyBlockedSkip},
	{
		label:   "ollama-webterminal-tailscale",
		fixture: "ollama-webterminal-tailscale.yaml",
		// Blocked by the operator-managed tailscale sidecar: it runs
		// `containerboot`, which exits when TS_AUTHKEY cannot join a tailnet.
		// The fixture ships a dummy auth key (no real ephemeral key is available
		// in CI), so the sidecar container never becomes Ready, the pod stays
		// NotReady, and the HermesInstance never reaches Ready=True. Unskip only
		// once a real ephemeral tailnet auth key is injected via secret in CI.
		// See #64. (#68, which blocked every other entry, is now fixed.)
		skip: "requires a live tailscale ephemeral auth key to reach Ready (dummy key cannot join a tailnet); see #64",
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
