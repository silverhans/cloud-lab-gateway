package pool

import "github.com/cloud-lab-gateway/gateway/internal/domain/shared"

type DomainEvent interface {
	Kind() string
}

type EventAllocated struct {
	ProjectID shared.ProjectID
	LabID     shared.LabInstanceID
}

func (EventAllocated) Kind() string { return "project.allocated" }

type EventReleasing struct {
	ProjectID shared.ProjectID
	LabID     shared.LabInstanceID
}

func (EventReleasing) Kind() string { return "project.releasing" }

type EventFreed struct {
	ProjectID shared.ProjectID
}

func (EventFreed) Kind() string { return "project.freed" }

type EventQuarantined struct {
	ProjectID shared.ProjectID
	Reason    string
	Failures  int
}

func (EventQuarantined) Kind() string { return "project.quarantined" }
