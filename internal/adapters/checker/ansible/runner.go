package ansible

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/verify"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

const (
	defaultCommand        = "ansible-playbook"
	defaultConnectTimeout = "10"
)

// Runner executes Ansible playbooks and parses the JSON callback output.
type Runner struct {
	Command string
}

var _ ports.CheckRunner = (*Runner)(nil)

// New returns a CheckRunner backed by ansible-playbook on PATH.
func New() *Runner {
	return &Runner{Command: defaultCommand}
}

// Run executes one playbook against a target host.
func (r *Runner) Run(ctx context.Context, req ports.CheckRequest) (ports.CheckResult, error) {
	defer zeroize(req.SSHPrivateKey)

	if req.PlaybookPath == "" || req.TargetHost == "" || req.SSHUser == "" {
		return ports.CheckResult{State: verify.StateErrored}, errors.New("ansible: playbook path, target host and ssh user are required")
	}

	runCtx := ctx
	cancel := func() {}
	if req.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
	}
	defer cancel()

	workDir, err := os.MkdirTemp("", "clg-ansible-*")
	if err != nil {
		return ports.CheckResult{State: verify.StateErrored}, fmt.Errorf("ansible: create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	keyPath := filepath.Join(workDir, "key")
	if err := os.WriteFile(keyPath, req.SSHPrivateKey, 0o600); err != nil {
		return ports.CheckResult{State: verify.StateErrored}, fmt.Errorf("ansible: write ssh key: %w", err)
	}

	inventoryPath := filepath.Join(workDir, "inventory.ini")
	inventory := fmt.Sprintf("[target]\n%s ansible_user=%s ansible_ssh_private_key_file=%s\n", req.TargetHost, req.SSHUser, keyPath)
	if err := os.WriteFile(inventoryPath, []byte(inventory), 0o600); err != nil {
		return ports.CheckResult{State: verify.StateErrored}, fmt.Errorf("ansible: write inventory: %w", err)
	}

	extraVars, err := json.Marshal(req.ExtraVars)
	if err != nil {
		return ports.CheckResult{State: verify.StateErrored}, fmt.Errorf("ansible: encode extra vars: %w", err)
	}

	args := []string{
		"-i", inventoryPath,
		req.PlaybookPath,
		"--extra-vars", string(extraVars),
		"--ssh-extra-args=-o StrictHostKeyChecking=accept-new -o ConnectTimeout=" + defaultConnectTimeout,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	// #nosec G204 -- ansible-playbook is invoked directly without a shell; playbook path comes from trusted CheckTemplate config.
	cmd := exec.CommandContext(runCtx, r.command(), args...)
	cmd.Env = append(os.Environ(),
		"ANSIBLE_STDOUT_CALLBACK=json",
		"ANSIBLE_CALLBACKS_ENABLED=json",
		"ANSIBLE_FORCE_COLOR=0",
		"ANSIBLE_HOST_KEY_CHECKING=False",
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return ports.CheckResult{State: verify.StateTimeout, Stdout: stdout.String()}, nil
	}

	parsed, parseErr := parseCallback(stdout.Bytes())
	if parseErr != nil {
		if runErr != nil {
			return ports.CheckResult{State: verify.StateErrored, Stdout: stdout.String()}, fmt.Errorf("ansible: run failed: %w: %s", runErr, stderr.String())
		}
		return ports.CheckResult{State: verify.StateErrored, Stdout: stdout.String()}, fmt.Errorf("ansible: parse json callback: %w", parseErr)
	}
	if runErr != nil && parsed.State == verify.StatePassed {
		parsed.State = verify.StateErrored
		return parsed, fmt.Errorf("ansible: run failed without failed tasks: %w: %s", runErr, stderr.String())
	}
	return parsed, nil
}

func (r *Runner) command() string {
	if r != nil && r.Command != "" {
		return r.Command
	}
	return defaultCommand
}

func zeroize(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}
