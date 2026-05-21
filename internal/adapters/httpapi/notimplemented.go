package httpapi

import (
	"context"
	"errors"
)

var errNotImplemented = errors.New("not implemented")

func (s *Server) notImplemented() error { return errNotImplemented }

func (s *Server) PostAuthLogin(context.Context, PostAuthLoginRequestObject) (PostAuthLoginResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetStreamLabs(context.Context, GetStreamLabsRequestObject) (GetStreamLabsResponseObject, error) {
	return nil, s.notImplemented()
}
