package httpapi

// Server implements the OpenAPI strict-server interface.
type Server struct {
	deps Deps
}

var _ StrictServerInterface = (*Server)(nil)

// NewServer creates a strict OpenAPI server implementation.
func NewServer(deps Deps) *Server {
	return &Server{deps: deps}
}
