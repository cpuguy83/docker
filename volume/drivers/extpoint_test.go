package drivers // import "github.com/docker/docker/volume/drivers"

import (
	"context"
	"testing"

	volumetestutils "github.com/docker/docker/volume/testutils"
)

func TestGetDriver(t *testing.T) {
	ctx := context.Background()
	s := NewStore(nil)
	_, err := s.GetDriver(ctx, "missing")
	if err == nil {
		t.Fatal("Expected error, was nil")
	}
	s.Register(volumetestutils.NewFakeDriver("fake"), "fake")

	d, err := s.GetDriver(ctx, "fake")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "fake" {
		t.Fatalf("Expected fake driver, got %s\n", d.Name())
	}
}
