package e2e

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hermes-operator e2e suite")
}

var execCommand = exec.Command

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)
	By("installing CRDs via helm chart")
	out, err := run("helm", "upgrade", "--install", "hermes-operator", "../../charts/hermes-operator",
		"--namespace", "hermes-system", "--create-namespace",
		"--set", "image.repository=hermes-operator",
		"--set", "image.tag=dev",
		"--set", "image.pullPolicy=IfNotPresent",
		"--wait", "--timeout=2m")
	Expect(err).ToNot(HaveOccurred(), "helm upgrade failed: %s", out)
})

func run(cmd string, args ...string) (string, error) {
	c := execCommand(cmd, args...)
	b, err := c.CombinedOutput()
	return string(b), err
}

func kubectl(args ...string) (string, error) {
	return run("kubectl", args...)
}

func runStdin(cmd string, args []string, stdin string) (string, error) {
	c := execCommand(cmd, args...)
	c.Stdin = strings.NewReader(stdin)
	b, err := c.CombinedOutput()
	return string(b), err
}
