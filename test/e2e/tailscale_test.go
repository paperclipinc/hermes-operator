package e2e

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The fixture uses a dummy auth key (tskey-dummy-not-a-real-key), so the
// tailscaled sidecar can never join a real tailnet and the pod may never
// report Ready. These specs therefore assert resource shape only and must
// not gate on readyReplicas. Real tailnet join is validated out-of-band
// (homelab, issue #42).
var _ = Describe("HermesInstance with Tailscale serve on kind", Ordered, func() {
	BeforeAll(func() {
		if os.Getenv("HERMES_E2E_FULL") != "1" {
			Skip("set HERMES_E2E_FULL=1 to enable the tailscale serve e2e (requires the agent image to be published)")
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
		// Eventually because no earlier spec waits for readiness, so the
		// reconciler may still be emitting resources when this runs.
		Eventually(func(g Gomega) {
			out, err := kubectl("get", "networkpolicy", "e2e-tailscale",
				"-n", "default",
				"-o", "jsonpath={.spec.egress[*].ports[*].port}")
			g.Expect(err).NotTo(HaveOccurred(), out)
			g.Expect(out).To(ContainSubstring("41641"))
		}).Should(Succeed())
	})
})
