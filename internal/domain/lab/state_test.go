package lab

import "testing"

func TestCanTransition_LegalMoves(t *testing.T) {
	t.Parallel()

	legal := []struct {
		from, to State
	}{
		{StatePendingQuota, StatePendingProject},
		{StatePendingQuota, StateRejected},
		{StatePendingProject, StateDeploying},
		{StatePendingProject, StateRejected},
		{StateDeploying, StateReady},
		{StateDeploying, StateFailed},
		{StateReady, StateChecking},
		{StateReady, StateFrozen},
		{StateReady, StateCleaning},
		{StateChecking, StateReady},
		{StateFrozen, StateReady},
		{StateFrozen, StateCleaning},
		{StateFailed, StateCleaning},
		{StateCleaning, StateDone},
		{StateCleaning, StateCleaning}, // self-loop: retry
	}

	for _, tc := range legal {
		tc := tc
		t.Run(string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			t.Parallel()
			if !CanTransition(tc.from, tc.to) {
				t.Errorf("expected %s → %s to be legal", tc.from, tc.to)
			}
		})
	}
}

func TestCanTransition_IllegalMoves(t *testing.T) {
	t.Parallel()

	illegal := []struct {
		from, to State
	}{
		{StatePendingQuota, StateDeploying}, // must go via PendingProject
		{StatePendingQuota, StateReady},     // skip deploy
		{StateDeploying, StateFrozen},       // can't freeze before ready
		{StateDeploying, StateCleaning},     // must go via Failed
		{StateReady, StateRejected},         // rejected is only for pre-deploy
		{StateReady, StateFailed},           // failed only from Deploying
		{StateChecking, StateCleaning},      // must return to Ready first
		{StateFrozen, StateDeploying},       // no re-deploy
		{StateDone, StateReady},             // terminal
		{StateDone, StateCleaning},          // terminal
		{StateRejected, StateDeploying},     // terminal
		{StateFailed, StateReady},           // can't recover, only cleanup
		{StatePendingProject, StateReady},   // skip deploy
		{StateReady, StateReady},            // identity
	}

	for _, tc := range illegal {
		tc := tc
		t.Run(string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			t.Parallel()
			if CanTransition(tc.from, tc.to) {
				t.Errorf("expected %s → %s to be illegal", tc.from, tc.to)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	t.Parallel()
	terminals := map[State]bool{
		StateDone:           true,
		StateRejected:       true,
		StatePendingQuota:   false,
		StatePendingProject: false,
		StateDeploying:      false,
		StateReady:          false,
		StateChecking:       false,
		StateFrozen:         false,
		StateFailed:         false,
		StateCleaning:       false,
	}
	for s, want := range terminals {
		s, want := s, want
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			if got := s.IsTerminal(); got != want {
				t.Errorf("%s.IsTerminal() = %v, want %v", s, got, want)
			}
		})
	}
}

func TestTransitionsTableCoversEveryState(t *testing.T) {
	t.Parallel()
	allStates := []State{
		StatePendingQuota, StatePendingProject, StateDeploying, StateReady,
		StateChecking, StateFrozen, StateFailed, StateCleaning, StateDone, StateRejected,
	}
	for _, s := range allStates {
		if _, ok := allowedTransitions[s]; !ok {
			t.Errorf("state %s is missing from allowedTransitions table", s)
		}
	}
}
