package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloud-lab-gateway/gateway/cmd/moodle-emulator/templates"
)

//go:embed templates/index.html
var templateFS embed.FS

type config struct {
	Addr          string
	GatewayURL    string
	Issuer        string
	PrivateKeyPEM string
	KID           string
}

type server struct {
	cfg    config
	signer *signer
	index  *template.Template
}

func main() {
	cfg := configFromEnv()
	signer, err := loadSigner(cfg.PrivateKeyPEM, cfg.KID)
	if err != nil {
		log.Fatalf("load signer: %v", err)
	}
	srv, err := newServer(cfg, signer)
	if err != nil {
		log.Fatalf("build server: %v", err)
	}
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("moodle emulator listening on %s", cfg.Addr)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func configFromEnv() config {
	port := env("EMULATOR_PORT", "9000")
	addr := port
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	return config{
		Addr:          addr,
		GatewayURL:    strings.TrimRight(env("EMULATOR_GATEWAY_URL", "http://localhost:8080"), "/"),
		Issuer:        env("EMULATOR_ISSUER", "https://moodle-emulator.local"),
		PrivateKeyPEM: os.Getenv("EMULATOR_PRIVATE_KEY_PEM_PATH"),
		KID:           env("EMULATOR_LTI_KID", "emu-key-1"),
	}
}

func newServer(cfg config, signer *signer) (*server, error) {
	index, err := template.ParseFS(templateFS, "templates/index.html")
	if err != nil {
		return nil, err
	}
	return &server{cfg: cfg, signer: signer, index: index}, nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /jwks.json", s.handleJWKS)
	mux.HandleFunc("POST /launch", s.handleLaunch)
	return mux
}

func (s *server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.index.Execute(w, map[string]interface{}{
		"Courses": templates.Courses,
		"Users":   templates.Users,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	raw, err := s.signer.jwksJSON()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

func (s *server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user, ok := findUser(r.FormValue("user"))
	if !ok {
		http.Error(w, "unknown user", http.StatusBadRequest)
		return
	}
	course, lab, ok := findLab(r.URL.Query().Get("course"), r.URL.Query().Get("lab"))
	if !ok {
		http.Error(w, "unknown course or lab", http.StatusBadRequest)
		return
	}

	launch, err := s.signer.issueLaunch(time.Now().UTC(), s.cfg.Issuer, user, course, lab)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := autoSubmitTemplate.Execute(w, map[string]string{
		"Action":  s.cfg.GatewayURL + "/lti/launch",
		"IDToken": launch.IDToken,
		"State":   launch.State,
		"Nonce":   launch.Nonce,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func findUser(externalID string) (templates.User, bool) {
	for _, user := range templates.Users {
		if user.ExternalID == externalID {
			return user, true
		}
	}
	return templates.User{}, false
}

func findLab(courseID, labSlug string) (templates.Course, templates.Lab, bool) {
	for _, course := range templates.Courses {
		if course.ExternalID != courseID {
			continue
		}
		for _, lab := range course.Labs {
			if lab.Slug == labSlug {
				return course, lab, true
			}
		}
	}
	return templates.Course{}, templates.Lab{}, false
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

var autoSubmitTemplate = template.Must(template.New("autosubmit").Parse(`<!doctype html>
<html lang="ru">
  <head>
    <meta charset="utf-8">
    <title>Launching lab...</title>
  </head>
  <body>
    <p>Отправляем LTI launch в Cloud Lab Gateway...</p>
    <form id="lti-launch" method="post" action="{{.Action}}">
      <input type="hidden" name="id_token" value="{{.IDToken}}">
      <input type="hidden" name="state" value="{{.State}}">
      <input type="hidden" name="nonce" value="{{.Nonce}}">
    </form>
    <script>document.getElementById("lti-launch").submit();</script>
    <noscript><button form="lti-launch" type="submit">Продолжить</button></noscript>
  </body>
</html>`))
