package httpapi

import "context"

func (s *Server) GetHealthz(context.Context, GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	return GetHealthz200Response{}, nil
}

func (s *Server) GetReadyz(context.Context, GetReadyzRequestObject) (GetReadyzResponseObject, error) {
	return GetReadyz200Response{}, nil
}
