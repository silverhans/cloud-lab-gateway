package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIndexListsCourses(t *testing.T) {
	t.Parallel()

	srv := testServer(t)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Moodle Emulator (demo)", "linux-101", "nginx-config-1", "student-001"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body does not contain %q", want)
		}
	}
}

func TestLaunchReturnsAutoSubmitForm(t *testing.T) {
	t.Parallel()

	srv := testServer(t)
	form := url.Values{"user": {"student-001"}}
	req := httptest.NewRequest(http.MethodPost, "/launch?course=linux-101&lab=linux-basics-1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"action=\"http://gateway.example/lti/launch\"", "name=\"id_token\"", "name=\"state\"", "name=\"nonce\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("body does not contain %q", want)
		}
	}
	if strings.Contains(body, "value=\"\"") {
		t.Fatal("auto-submit form contains empty hidden value")
	}
}

func TestJWKSReturnsPublicKey(t *testing.T) {
	t.Parallel()

	srv := testServer(t)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/jwks.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	raw, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(raw), "\"kid\":\"test-kid\"") {
		t.Fatalf("jwks does not include test kid: %s", raw)
	}
}

func testServer(t *testing.T) *server {
	t.Helper()

	srv, err := newServer(config{
		GatewayURL: "http://gateway.example",
		Issuer:     "https://moodle-emulator.local",
		KID:        "test-kid",
	}, testSigner(t))
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return srv
}
