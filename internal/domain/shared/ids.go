// Package shared contains value objects, ID types and errors used across
// all domain contexts. It must not import any other domain package.
package shared

import (
	"github.com/google/uuid"
)

// Strongly-typed UUIDs prevent accidental cross-aggregate misuse at compile time.
type (
	UserID        uuid.UUID
	CourseID      uuid.UUID
	EnrollmentID  uuid.UUID
	ProjectID     uuid.UUID
	LabInstanceID uuid.UUID
	LabTemplateID uuid.UUID
	CheckTemplate uuid.UUID
	CheckRunID    uuid.UUID
	AuditEventID  uuid.UUID
	SecretID      uuid.UUID
)

func NewLabInstanceID() LabInstanceID { return LabInstanceID(uuid.New()) }
func NewProjectID() ProjectID         { return ProjectID(uuid.New()) }
func NewCheckRunID() CheckRunID       { return CheckRunID(uuid.New()) }
func NewAuditEventID() AuditEventID   { return AuditEventID(uuid.New()) }
func NewSecretID() SecretID           { return SecretID(uuid.New()) }
func NewUserID() UserID               { return UserID(uuid.New()) }

func (id LabInstanceID) String() string { return uuid.UUID(id).String() }
func (id ProjectID) String() string     { return uuid.UUID(id).String() }
func (id UserID) String() string        { return uuid.UUID(id).String() }
func (id CourseID) String() string      { return uuid.UUID(id).String() }
func (id CheckRunID) String() string    { return uuid.UUID(id).String() }
