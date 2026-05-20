//go:build integration

package ansible

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cloud-lab-gateway/gateway/internal/domain/verify"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

func TestRunnerRunLocalhost(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath(defaultCommand); err != nil {
		t.Skip("ansible-playbook is not installed")
	}

	dir := t.TempDir()
	playbook := filepath.Join(dir, "local.yml")
	if err := os.WriteFile(playbook, []byte(`---
- name: local smoke
  hosts: localhost
  connection: local
  gather_facts: false
  tasks:
    - name: assert local execution works
      ansible.builtin.assert:
        that:
          - true
        success_msg: local ok
`), 0o600); err != nil {
		t.Fatalf("write playbook: %v", err)
	}

	key := []byte("unused-for-local-connection")
	result, err := New().Run(context.Background(), ports.CheckRequest{
		PlaybookPath:   playbook,
		TargetHost:     "localhost",
		SSHUser:        "root",
		SSHPrivateKey:  key,
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("run ansible: %v", err)
	}
	if result.State != verify.StatePassed {
		t.Fatalf("state = %q, want %q; stdout=%s", result.State, verify.StatePassed, result.Stdout)
	}
	for i, b := range key {
		if b != 0 {
			t.Fatalf("key byte %d was not zeroized", i)
		}
	}
}
