package moodle

import (
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// Deps contains collaborators for LMS/LTI launch use cases.
type Deps struct {
	LMS     ports.LMSProvider
	UoW     ports.UnitOfWork
	Users   ports.UserRepo
	Courses ports.CourseRepo
	Now     func() time.Time
}

func (d Deps) now() time.Time {
	if d.Now == nil {
		return time.Now().UTC()
	}
	return d.Now()
}
