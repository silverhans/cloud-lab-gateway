// Package lab is the Lab Lifecycle bounded context. It owns LabInstance and
// its state machine. See docs/STATE_MACHINES.md for the full diagram.
package lab

import "github.com/cloud-lab-gateway/gateway/internal/domain/shared"

// State enumerates LabInstance states.
type State string

const (
	StatePendingQuota   State = "pending_quota"
	StatePendingProject State = "pending_project"
	StateDeploying      State = "deploying"
	StateReady          State = "ready"
	StateChecking       State = "checking"
	StateFrozen         State = "frozen"
	StateFailed         State = "failed"
	StateCleaning       State = "cleaning"
	StateDone           State = "done"
	StateRejected       State = "rejected"
)

// IsTerminal reports states that have no outgoing transitions.
func (s State) IsTerminal() bool {
	return s == StateDone || s == StateRejected
}

// allowedTransitions is the source of truth for the lab state machine. Each
// outgoing transition is documented in docs/STATE_MACHINES.md.
var allowedTransitions = map[State]map[State]struct{}{
	StatePendingQuota:   set(StateRejected, StatePendingProject),
	StatePendingProject: set(StateRejected, StateDeploying),
	StateDeploying:      set(StateReady, StateFailed),
	StateReady:          set(StateChecking, StateFrozen, StateCleaning),
	StateChecking:       set(StateReady),
	StateFrozen:         set(StateReady, StateCleaning),
	StateFailed:         set(StateCleaning),
	StateCleaning:       set(StateDone, StateCleaning), // self-loop = retry
	StateDone:           {},                            // terminal
	StateRejected:       {},                            // terminal
}

func set(states ...State) map[State]struct{} {
	m := make(map[State]struct{}, len(states))
	for _, s := range states {
		m[s] = struct{}{}
	}
	return m
}

// CanTransition reports whether from → to is a legal transition. Self-loops
// are allowed only when the transitions table explicitly lists them (currently
// only StateCleaning → StateCleaning for cleanup retries).
func CanTransition(from, to State) bool {
	allowed, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, found := allowed[to]
	return found
}

// MustTransition panics if the transition is illegal. Use only from tests or
// migration code; production code must use LabInstance.Transition.
func MustTransition(from, to State) {
	if !CanTransition(from, to) {
		panic(shared.ErrInvalidTransition{Entity: "lab", From: string(from), To: string(to)})
	}
}
