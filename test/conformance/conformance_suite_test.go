package conformance

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Conformance suite: categories live in sibling files:
//   - negative_test.go             webhook deny paths
//   - idempotency_test.go          10-reconcile no-op canary
//   - upgrade_test.go              prior-release -> HEAD matrix
//   - gitops_test.go               FluxCD SSA + SelfConfig no-flap
//   - failure_injection_test.go    SIGKILL mid-reconcile
func TestConformance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hermes-operator conformance suite")
}

var (
	suiteCtx    context.Context
	suiteCancel context.CancelFunc
)

var _ = BeforeSuite(func() {
	suiteCtx, suiteCancel = context.WithCancel(context.Background())
	SetDefaultEventuallyTimeout(5 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)
	// The suite needs a live cluster. Resolve the kubeconfig the same way the
	// rest of the suite does (clientcmdPath in helpers.go): prefer $KUBECONFIG,
	// otherwise fall back to ~/.kube/config. Only skip when neither is present.
	//
	// Historically this checked os.Getenv("KUBECONFIG") != "" directly, which
	// made the whole suite silently SKIP in CI because helm/kind-action writes
	// the kubeconfig to ~/.kube/config but never exports KUBECONFIG (#64). CI
	// now exports KUBECONFIG explicitly; this fallback is defense-in-depth so a
	// reachable cluster is never silently ignored again.
	kubeconfig := clientcmdPath()
	if _, err := os.Stat(kubeconfig); err != nil {
		Skip(fmt.Sprintf(
			"no kubeconfig at %q (set KUBECONFIG): conformance suite requires a live kind cluster with the operator installed",
			kubeconfig))
	}
})

var _ = AfterSuite(func() {
	if suiteCancel != nil {
		suiteCancel()
	}
})
