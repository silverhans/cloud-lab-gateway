package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	appmoodle "github.com/cloud-lab-gateway/gateway/internal/app/moodle"
	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

const defaultSessionTTL = 8 * time.Hour

// LTIDeps bundles the browser LTI endpoint collaborators.
type LTIDeps struct {
	Moodle        MoodleLaunchService
	Logger        *zap.Logger
	DevMode       bool
	SessionSecret string
	SessionTTL    time.Duration
}

type ltiServer struct {
	deps     LTIDeps
	problems *Server
}

// NewLTIMux builds the /lti mux for browser Resource Link launches.
func NewLTIMux(deps LTIDeps) http.Handler {
	r := chi.NewRouter()
	s := &ltiServer{
		deps: deps,
		problems: NewServer(Deps{
			Logger:        deps.Logger,
			DevMode:       deps.DevMode,
			SessionSecret: deps.SessionSecret,
		}),
	}
	r.Post("/launch", s.handleLaunch)
	return r
}

func (s *ltiServer) handleLaunch(w http.ResponseWriter, r *http.Request) {
	if s.deps.Moodle == nil {
		s.problems.renderProblem(w, r, shared.ErrLMSUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.problems.renderProblem(w, r, shared.ErrInvalidInput)
		return
	}
	result, err := s.deps.Moodle.HandleLaunch(r.Context(), appmoodle.LaunchInput{
		IDToken: r.FormValue("id_token"),
		State:   r.FormValue("state"),
		Nonce:   r.FormValue("nonce"),
	})
	if err != nil {
		s.problems.renderProblem(w, r, err)
		return
	}
	if result == nil || result.User == nil {
		s.problems.renderProblem(w, r, shared.ErrLMSUnavailable)
		return
	}
	ttl := s.deps.SessionTTL
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	if err := issueSession(w, Principal{
		UserID:      uuid.UUID(result.User.ID),
		DisplayName: result.User.DisplayName,
		Email:       result.User.Email,
		Role:        string(result.User.Role),
		CourseRoles: httpCourseRoles(result.CourseRoles),
	}, ttl, s.deps.SessionSecret, s.deps.DevMode); err != nil {
		s.problems.renderProblem(w, r, err)
		return
	}
	redirectPath := result.RedirectPath
	if redirectPath == "" {
		redirectPath = "/"
	}
	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}

func httpCourseRoles(in map[shared.CourseID]identity.CourseRole) map[uuid.UUID]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[uuid.UUID]string, len(in))
	for id, role := range in {
		out[uuid.UUID(id)] = string(role)
	}
	return out
}
