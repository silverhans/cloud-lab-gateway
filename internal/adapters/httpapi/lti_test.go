package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appmoodle "github.com/cloud-lab-gateway/gateway/internal/app/moodle"
	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/google/uuid"
)

type fakeMoodleLaunchService struct {
	result *appmoodle.LaunchResult
	err    error
	got    appmoodle.LaunchInput
}

func (s *fakeMoodleLaunchService) HandleLaunch(_ context.Context, in appmoodle.LaunchInput) (*appmoodle.LaunchResult, error) {
	s.got = in
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func TestLTILaunchIssuesSessionAndRedirects(t *testing.T) {
	t.Parallel()
	userID := shared.UserID(uuid.New())
	courseID := shared.CourseID(uuid.New())
	fake := &fakeMoodleLaunchService{result: &appmoodle.LaunchResult{
		User: &identity.User{
			ID:          userID,
			DisplayName: "Иван Петров",
			Email:       "student-001@emulator.local",
			Role:        identity.RoleStudent,
		},
		CourseRoles:  map[shared.CourseID]identity.CourseRole{courseID: identity.CourseRoleLearner},
		RedirectPath: "/student",
	}}
	h := NewLTIMux(LTIDeps{
		Moodle:        fake,
		DevMode:       true,
		SessionSecret: testJWTSecret,
		SessionTTL:    time.Hour,
	})

	form := url.Values{
		"id_token": {"signed.jwt"},
		"state":    {"state-1"},
		"nonce":    {"nonce-1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/launch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != "/student" {
		t.Fatalf("location = %q", rr.Header().Get("Location"))
	}
	if fake.got.IDToken != "signed.jwt" || fake.got.State != "state-1" || fake.got.Nonce != "nonce-1" {
		t.Fatalf("unexpected launch input: %+v", fake.got)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want one session cookie", len(cookies))
	}
	parseReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", http.NoBody)
	parseReq.AddCookie(cookies[0])
	principal, err := NewServer(Deps{SessionSecret: testJWTSecret}).parsePrincipal(parseReq)
	if err != nil {
		t.Fatalf("parse principal: %v", err)
	}
	if principal.UserID != uuid.UUID(userID) || principal.DisplayName != "Иван Петров" || principal.Email != "student-001@emulator.local" {
		t.Fatalf("principal mismatch: %+v", principal)
	}
	if principal.CourseRoles[uuid.UUID(courseID)] != string(identity.CourseRoleLearner) {
		t.Fatalf("course roles mismatch: %+v", principal.CourseRoles)
	}
}

func TestLTILaunchRendersProblem(t *testing.T) {
	t.Parallel()
	fake := &fakeMoodleLaunchService{err: shared.ErrUnauthorized}
	h := NewLTIMux(LTIDeps{Moodle: fake, DevMode: true, SessionSecret: testJWTSecret})
	req := httptest.NewRequest(http.MethodPost, "/launch", strings.NewReader("id_token=bad"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}
