// Package identity is the Identity & Access bounded context.
// It owns User, Course, Enrollment and RBAC policy primitives.
package identity

import (
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

type Role string

const (
	RoleStudent Role = "student"
	RoleTeacher Role = "teacher"
	RoleAdmin   Role = "admin"
)

type CourseRole string

const (
	CourseRoleLearner CourseRole = "learner"
	CourseRoleTeacher CourseRole = "teacher"
)

type User struct {
	ID          shared.UserID
	DisplayName string
	Email       string
	Role        Role
	CreatedAt   time.Time
}

// LTIIdentity links a User to an LMS-side identity (Moodle iss+sub).
// Composite unique on (Iss, Sub).
type LTIIdentity struct {
	UserID    shared.UserID
	Iss       string // LMS issuer URL
	Sub       string // LTI subject claim
	CreatedAt time.Time
}

type Course struct {
	ID         shared.CourseID
	ExternalID string // Moodle course_id
	Name       string
	KIDomainID string // 1 course = 1 КИ domain (по условиям кейса)
	CreatedAt  time.Time
}

type Enrollment struct {
	UserID       shared.UserID
	CourseID     shared.CourseID
	RoleInCourse CourseRole
	CreatedAt    time.Time
}
