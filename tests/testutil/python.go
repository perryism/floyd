// Package testutil provides shared helpers for the contract/integration/unit test suites.
package testutil

import (
	"os"
	"os/exec"
	"testing"
)

// PythonExecutable returns the Python interpreter to launch handler workers with during
// tests: $GATEWAY_TEST_PYTHON if set (for local development against a project venv), else
// "python3" on $PATH. It fails the test immediately (not Skip) if that interpreter can't
// import gateway_handler_sdk — per this project's constitution, the Testing Standards
// principle is NON-NEGOTIABLE, so a missing dependency must fail CI loudly rather than
// silently skip the tests that exercise the real Go<->Python contract.
func PythonExecutable(t *testing.T) string {
	t.Helper()

	python := os.Getenv("GATEWAY_TEST_PYTHON")
	if python == "" {
		python = "python3"
	}

	cmd := exec.Command(python, "-c", "import gateway_handler_sdk")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("python interpreter %q cannot import gateway_handler_sdk (set GATEWAY_TEST_PYTHON "+
			"to a venv with `pip install -e ./handlers-sdk` run): %v\n%s", python, err, out)
	}
	return python
}
