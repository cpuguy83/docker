package httputils

import (
	"fmt"
	"net/http"

	"github.com/cpuguy83/errclass/status"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	grpcstatus "google.golang.org/grpc/status"
)

// GetHTTPErrorStatusCode retrieves status code from error message.
func GetHTTPErrorStatusCode(err error) int {
	if err == nil {
		logrus.WithFields(logrus.Fields{"error": err}).Error("unexpected HTTP error handling")
		return http.StatusInternalServerError
	}

	code, ok := status.HTTPCode(err)
	if ok {
		return code
	}

	s, ok := grpcstatus.FromError(err)
	if !ok {
		logrus.WithFields(logrus.Fields{
			"module":     "api",
			"error_type": fmt.Sprintf("%T", err),
		}).Debugf("FIXME: Got an API for which error does not match any expected type!!!: %+v", err)
	}

	code, _ = status.HTTPCode(status.FromGRPC(s))
	return code
}

func apiVersionSupportsJSONErrors(version string) bool {
	const firstAPIVersionWithJSONErrors = "1.23"
	return version == "" || versions.GreaterThan(version, firstAPIVersionWithJSONErrors)
}

// MakeErrorHandler makes an HTTP handler that decodes a Docker error and
// returns it in the response.
func MakeErrorHandler(err error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statusCode := GetHTTPErrorStatusCode(err)
		vars := mux.Vars(r)
		if apiVersionSupportsJSONErrors(vars["version"]) {
			response := &types.ErrorResponse{
				Message: err.Error(),
			}
			WriteJSON(w, statusCode, response)
		} else {
			http.Error(w, grpc.ErrorDesc(err), statusCode)
		}
	}
}
