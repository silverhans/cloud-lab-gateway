package moodle

import "context"

// Service exposes Moodle/LTI use cases as methods for thin adapters.
type Service struct {
	deps Deps
}

// NewService creates a Moodle application service.
func NewService(deps Deps) Service {
	return Service{deps: deps}
}

// HandleLaunch verifies an LTI launch and materialises the browser session data.
func (s Service) HandleLaunch(ctx context.Context, in LaunchInput) (*LaunchResult, error) {
	return HandleLaunch(ctx, s.deps, in)
}
