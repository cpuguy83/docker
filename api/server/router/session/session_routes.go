package session

import (
	"net/http"

	"github.com/cpuguy83/errclass"
	"golang.org/x/net/context"
)

func (sr *sessionRouter) startSession(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) error {
	err := sr.backend.HandleHTTPRequest(ctx, w, r)
	if err != nil {
		return errclass.InvalidArgument(err)
	}
	return nil
}
