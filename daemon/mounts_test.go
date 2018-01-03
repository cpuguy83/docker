package daemon

import (
	"sync"
	"testing"

	"github.com/pkg/errors"
)

func TestMountRegister(t *testing.T) {
	r := &mountRegister{root: "/"}

	expectError := errors.New("some error")
	mounts := make(map[string]int)
	var mu sync.Mutex

	getCount := func(p string) int {
		mu.Lock()
		defer mu.Unlock()
		return mounts[p]
	}

	var mountIter int
	mountFn := func(p string) error {
		mu.Lock()
		defer mu.Unlock()
		mountIter++
		if mountIter == 1 {
			return expectError
		}
		mounts[p]++
		return nil
	}

	unmountFn := func(p string) error {
		mu.Lock()
		defer mu.Unlock()
		mounts[p]--
		return nil
	}

	paths := make(map[string]bool)

	// check the error case, which we are expecting on the first iteration due
	// to our mountFn implementation
	if _, err := r.Get("apple", "pie", mountFn); err != expectError {
		t.Fatalf("expected error %q, got: %+v", expectError, err)
	}

	if _, err := r.Get("apple", "pie", mountFn); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get("apple", "pipe", unmountFn); err != expectError {
		t.Fatalf("expected error %q, got: %+v", expectError, err)
	}
	if _, err := r.Get("apple", "pipe", unmountFn); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ ref, id string }{
		{"apple", "pie"},
		{"apple", "cobbler"},
		{"peach", "pie"},
		{"peach", "cobbler"},
		{"strawberry", "shortcake"},
		{"banana", "pudding"},
		{"banana", "pie"},
		{"coconut", "cream"},
	} {
		t.Run(c.ref+"_"+c.id, func(t *testing.T) {
			t.Parallel() // make sure the race detector has something to work with

			p, err := r.Get(c.ref, c.id, mountFn)
			if err != nil {
				t.Fatal(err)
			}
			if p == "" {
				t.Fatal("returned path should not be empty")
			}
			if paths[p] {
				t.Fatal("got unexpected duplicate path")
			}
			if getCount(p) != 1 {
				t.Fatal("expected 1 mount, got %d", getCount(p))
			}

			p2, err := r.Get(c.ref, c.id, mountFn)
			if err != nil {
				t.Fatal(err)
			}
			if p2 != p {
				t.Fatalf("expected %q, got: %v", p, p2)
			}

			// There's two references currently, so our mount count should not have changed
			if err := r.Release(c.ref, c.id, unmountFn); err != nil {
				t.Fatal(err)
			}
			if getCount(p) != 1 {
				t.Fatalf("expected 1 mount, got %d", getCount(p))
			}

			// Now there is only the 1 reference left, we should see our mounts decreaes
			if err := r.Release(c.ref, c.id, unmountFn); err != nil {
				t.Fatal(err)
			}
			if getCount(p) != 0 {
				t.Fatal("expected 0 mounts, got %d", getCount(p))
			}

			// sanity check, we should be able to get a new mount reference now
			// leave this in place for the next test run
			p3, err := r.Get(c.ref, c.id, mountFn)
			if err != nil {
				t.Fatal(err)
			}
			if getCount(p3) != 1 {
				t.Fatalf("expected 1 mount, got %d", getCount(p3))
			}
		})
	}
}
