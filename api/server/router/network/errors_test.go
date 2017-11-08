package network

import (
	"testing"

	"github.com/cpuguy83/errclass"
)

func TestAmbigousResultsError_InvalidArgument(t *testing.T) {
	e := ambigousResultsError("foo")
	if !errclass.IsInvalidArgument(e) {
		t.Fatal(`"ambiguousResultsError" should implement errclass.InvalidArgument`)
	}
}

func TestInvalidFilter_InvalidArgument(t *testing.T) {
	if !errclass.IsInvalidArgument(invalidFilter("foo")) {
		t.Fatal(`"invalidFilter" error should implement errclass.InvalidArgument`)
	}
}

func TestNameConflict(t *testing.T) {
	e := nameConflict("foo")
	if !errclass.IsConflict(e) {
		t.Fatal("expected nameConflict to return error that implements errclass.Conflict")
	}
}
