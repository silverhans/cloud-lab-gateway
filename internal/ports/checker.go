package ports

import (
	"context"

	"github.com/cloud-lab-gateway/gateway/internal/domain/verify"
)

// CheckRunner executes Ansible playbooks against a deployed lab VM and parses
// the JSON callback into structured CheckRun results.
type CheckRunner interface {
	// Run executes the given playbook and returns the structured result.
	// The implementation MUST:
	//   - decrypt and use the provided SSH key only in-memory
	//   - write the inventory to a temp file with 0600 mode
	//   - run with --json callback and ANSIBLE_FORCE_COLOR=0
	//   - enforce the timeout via context cancellation
	//   - zeroize the SSH private key after the run
	Run(ctx context.Context, req CheckRequest) (CheckResult, error)
}

// CheckRequest describes a single playbook execution.
type CheckRequest struct {
	PlaybookPath   string
	TargetHost     string // IP or hostname (floating IP)
	SSHUser        string
	SSHPrivateKey  []byte // PEM, will be zeroized
	ExtraVars      map[string]string
	TimeoutSeconds int
}

// CheckResult is the structured result returned by the runner.
type CheckResult struct {
	State  verify.RunState
	Steps  []verify.StepResult
	Stdout string
	Stats  AnsibleStats
}

// AnsibleStats mirrors the `stats` section of Ansible's JSON output.
type AnsibleStats struct {
	OK          int
	Changed     int
	Failed      int
	Unreachable int
	Skipped     int
}
