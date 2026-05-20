package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	applab "github.com/cloud-lab-gateway/gateway/internal/app/lab"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/google/uuid"
)

const testJWTSecret = "test-secret-at-least-long-enough"

type fakeLabService struct {
	result *labdomain.LabInstance
	err    error
	got    applab.CreateInput
}

func (s *fakeLabService) CreateLab(_ context.Context, in applab.CreateInput) (*labdomain.LabInstance, error) {
	s.got = in
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func TestPostLabsHappyPath(t *testing.T) {
	t.Parallel()
	studentID := uuid.New()
	courseID := uuid.New()
	templateID := uuid.New()
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	fake := &fakeLabService{result: labdomain.New(
		shared.NewLabInstanceID(),
		shared.UserID(studentID),
		shared.CourseID(courseID),
		shared.LabTemplateID(templateID),
		now,
	)}
	h := NewMux(testDeps(fake))
	body := map[string]string{
		"course_id":       courseID.String(),
		"lab_template_id": templateID.String(),
		"student_user_id": uuid.NewString(),
	}
	req := authedJSONRequest(t, http.MethodPost, "/labs", body, Principal{UserID: studentID, Role: "student"})
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if fake.got.StudentUserID != shared.UserID(studentID) {
		t.Fatalf("StudentUserID = %s, want principal %s", fake.got.StudentUserID, studentID)
	}
	var got LabInstance
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.StudentUserId != studentID || got.CourseId != courseID || got.LabTemplateId != templateID {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestPostLabsQuotaExceededProblem(t *testing.T) {
	t.Parallel()
	fake := &fakeLabService{err: shared.ErrQuotaExceeded}
	h := NewMux(testDeps(fake))
	req := authedJSONRequest(t, http.MethodPost, "/labs", validLabCreateBody(), Principal{UserID: uuid.New(), Role: "student"})
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content-type = %q", got)
	}
	problem := decodeProblem(t, rr.Body.Bytes())
	if problem.Code == nil || *problem.Code != "ERR_QUOTA_EXCEEDED" {
		t.Fatalf("problem code = %#v", problem.Code)
	}
}

func TestPostLabsRequiresValidSession(t *testing.T) {
	t.Parallel()
	h := NewMux(testDeps(&fakeLabService{}))

	for name, cookie := range map[string]*http.Cookie{
		"missing": nil,
		"tampered": func() *http.Cookie {
			c := sessionCookie(t, Principal{UserID: uuid.New(), Role: "student"})
			c.Value += "tamper"
			return c
		}(),
	} {
		name, cookie := name, cookie
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := jsonRequest(t, http.MethodPost, "/labs", validLabCreateBody())
			if cookie != nil {
				req.AddCookie(cookie)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestPostLabsMalformedBody(t *testing.T) {
	t.Parallel()
	h := NewMux(testDeps(&fakeLabService{}))
	req := httptest.NewRequest(http.MethodPost, "/labs", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie(t, Principal{UserID: uuid.New(), Role: "student"}))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestNotImplementedEndpoint(t *testing.T) {
	t.Parallel()
	h := NewMux(testDeps(&fakeLabService{}))
	req := authedJSONRequest(t, http.MethodGet, "/labs", nil, Principal{UserID: uuid.New(), Role: "student"})
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	problem := decodeProblem(t, rr.Body.Bytes())
	if problem.Code == nil || *problem.Code != "ERR_NOT_IMPLEMENTED" {
		t.Fatalf("problem code = %#v", problem.Code)
	}
}

func TestIssueSessionThenRequireAuth(t *testing.T) {
	t.Setenv("CLG_JWT_SECRET", testJWTSecret)
	p := Principal{UserID: uuid.New(), Role: "teacher", CourseRoles: map[uuid.UUID]string{uuid.New(): "teacher"}}
	rr := httptest.NewRecorder()
	if err := IssueSession(rr, p, time.Hour); err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected session cookie, got %d", len(cookies))
	}

	h := NewServer(testDeps(&fakeLabService{}))
	req := httptest.NewRequest(http.MethodGet, "/auth/me", http.NoBody)
	req.AddCookie(cookies[0])
	authRR := httptest.NewRecorder()
	var passed Principal
	h.RequireAuth(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var ok bool
		passed, ok = principalFrom(r.Context())
		if !ok {
			t.Fatal("principal missing from context")
		}
	})).ServeHTTP(authRR, req)
	if authRR.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", authRR.Code, authRR.Body.String())
	}
	if passed.UserID != p.UserID || passed.Role != p.Role || len(passed.CourseRoles) != len(p.CourseRoles) {
		t.Fatalf("principal mismatch: got %+v want %+v", passed, p)
	}

	tampered := *cookies[0]
	tampered.Value += "x"
	req = httptest.NewRequest(http.MethodGet, "/auth/me", http.NoBody)
	req.AddCookie(&tampered)
	authRR = httptest.NewRecorder()
	h.RequireAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("tampered cookie should not pass")
	})).ServeHTTP(authRR, req)
	if authRR.Code != http.StatusUnauthorized {
		t.Fatalf("tampered status = %d, body = %s", authRR.Code, authRR.Body.String())
	}
}

func testDeps(fake *fakeLabService) Deps {
	return Deps{Lab: fake, DevMode: true, SessionSecret: testJWTSecret}
}

func authedJSONRequest(t *testing.T, method, path string, body interface{}, p Principal) *http.Request {
	t.Helper()
	req := jsonRequest(t, method, path, body)
	req.AddCookie(sessionCookie(t, p))
	return req
}

func jsonRequest(t *testing.T, method, path string, body interface{}) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func sessionCookie(t *testing.T, p Principal) *http.Cookie {
	t.Helper()
	rr := httptest.NewRecorder()
	if err := issueSession(rr, p, time.Hour, testJWTSecret, true); err != nil {
		t.Fatalf("issue session: %v", err)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	return cookies[0]
}

func validLabCreateBody() map[string]string {
	return map[string]string{
		"course_id":       uuid.NewString(),
		"lab_template_id": uuid.NewString(),
	}
}

func decodeProblem(t *testing.T, payload []byte) Problem {
	t.Helper()
	var p Problem
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("decode problem: %v; body=%s", err, string(payload))
	}
	if p.Type == "" || p.Title == "" || p.Status == 0 || p.Code == nil {
		t.Fatalf("incomplete problem: %+v", p)
	}
	return p
}

func TestProblemMetaDoesNotExposeInternalErrors(t *testing.T) {
	t.Parallel()
	status, _, detail, code := problemMeta(errors.New("database password is bad"))
	if status != http.StatusInternalServerError || detail != "internal server error" || code != "ERR_INTERNAL" {
		t.Fatalf("unexpected internal problem meta: status=%d detail=%q code=%q", status, detail, code)
	}
}
