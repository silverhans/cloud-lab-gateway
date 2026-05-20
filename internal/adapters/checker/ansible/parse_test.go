package ansible

import (
	"os"
	"testing"

	"github.com/cloud-lab-gateway/gateway/internal/domain/verify"
)

func TestParseCallbackPass(t *testing.T) {
	t.Parallel()

	raw := readFixture(t, "testdata/callback_pass.json")
	result, err := parseCallback(raw)
	if err != nil {
		t.Fatalf("parse callback: %v", err)
	}
	if result.State != verify.StatePassed {
		t.Fatalf("state = %q, want %q", result.State, verify.StatePassed)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("steps len = %d, want 3", len(result.Steps))
	}
	if result.Steps[1].Status != verify.StepChanged {
		t.Fatalf("second step status = %q, want changed", result.Steps[1].Status)
	}
	if result.Stats.OK != 3 || result.Stats.Changed != 1 || result.Stats.Failed != 0 {
		t.Fatalf("unexpected stats: %+v", result.Stats)
	}
}

func TestParseCallbackFail(t *testing.T) {
	t.Parallel()

	raw := readFixture(t, "testdata/callback_fail.json")
	result, err := parseCallback(raw)
	if err != nil {
		t.Fatalf("parse callback: %v", err)
	}
	if result.State != verify.StateFailed {
		t.Fatalf("state = %q, want %q", result.State, verify.StateFailed)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(result.Steps))
	}
	if result.Steps[1].Status != verify.StepFailed {
		t.Fatalf("second step status = %q, want failed", result.Steps[1].Status)
	}
	if result.Steps[1].Message == "" {
		t.Fatal("failed step message is empty")
	}
	if result.Stats.Failed != 1 {
		t.Fatalf("failed stats = %d, want 1", result.Stats.Failed)
	}
}

func TestParseCallbackRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	result, err := parseCallback([]byte("not json"))
	if err == nil {
		t.Fatal("expected error")
	}
	if result.State != verify.StateErrored {
		t.Fatalf("state = %q, want %q", result.State, verify.StateErrored)
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return raw
}
