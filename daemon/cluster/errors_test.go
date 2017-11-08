package cluster

import (
	"testing"

	"github.com/cpuguy83/errclass"
)

func TestInvalidUnlockKey_Unauthorized(t *testing.T) {
	e := invalidUnlockKey{}
	if !errclass.IsUnauthorized(e) {
		t.Fatalf("%T should implement errclass.ErrUnauthorized", e)
	}
}

func TestNotAvailableError_Unavailable(t *testing.T) {
	e := notAvailableError("foo")
	if !errclass.IsUnavailable(e) {
		t.Fatalf("%T should implement errclass.ErrUnavailable", e)
	}
}

func TestNotLockedError_Conflict(t *testing.T) {
	e := notLockedError{}
	if !errclass.IsConflict(e) {
		t.Fatalf("%T should implement errclass.ErrConflict", e)
	}
}
