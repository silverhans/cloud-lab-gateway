package ports

import (
	"context"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// LMSProvider abstracts Moodle access. Two adapters implement it:
//   - moodlerest:   classic REST-WS calls (token-based)
//   - lti13:        LTI 1.3 launch + Names and Roles Service + Assignment and Grade Service
type LMSProvider interface {
	// VerifyLaunch verifies an LTI 1.3 id_token (JWT signed by the LMS).
	// Returns the decoded launch claims on success.
	VerifyLaunch(ctx context.Context, idToken, state, nonce string) (*LTILaunch, error)

	// ReportGrade sends a score back to the LMS for a given AGS lineitem.
	// Optional — used after a check.passed event.
	ReportGrade(ctx context.Context, launch *LTILaunch, score float64, maxScore float64, comment string) error

	// GetCourseMembers fetches the roster via the NRPS (Names and Roles) service.
	GetCourseMembers(ctx context.Context, courseExternalID string) ([]LMSMember, error)
}

// LTILaunch carries the verified claims of an LTI 1.3 resource link launch.
type LTILaunch struct {
	Iss              string
	Sub              string
	Aud              []string
	Email            string
	Name             string
	CourseExternalID string
	ResourceLinkID   string
	RolesInContext   []string // urn:lti:role:ims/lis/{Learner|Instructor|...}
	AGSEndpoint      *AGSEndpoint
	NRPSEndpoint     string
	Raw              map[string]any
}

// AGSEndpoint is the Assignment & Grade Service endpoint negotiated at launch.
type AGSEndpoint struct {
	LineItemsURL string
	LineItemURL  string
	Scope        []string
}

// LMSMember describes a user as returned by NRPS.
type LMSMember struct {
	LtiSub      string
	Email       string
	DisplayName string
	Roles       []string
}

// MoodleEmulator is a thin variant of LMSProvider used by the demo emulator.
// Defined here so demo code can ship behind a build tag.
type MoodleEmulator interface {
	// IssueLaunch creates a signed launch payload for the given user/course.
	IssueLaunch(ctx context.Context, userExternalID, courseExternalID string) (idToken, state, nonce string, err error)
}

// UserMapper resolves an LTI identity to our internal User. Implementations
// must use the (iss, sub) tuple as the primary key — never the email.
type UserMapper interface {
	UpsertFromLaunch(ctx context.Context, launch *LTILaunch) (shared.UserID, error)
}
