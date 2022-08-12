package queue

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestUniqueue(t *testing.T) {
	var q Unique

	const key = "a"

	q.Add(key)
	v, shutdown := q.Get()
	assert.Assert(t, !shutdown)
	assert.Equal(t, v, key)
	q.Done(key)

	type getResult struct {
		v        string
		shutdown bool
	}

	ch := make(chan getResult, 1)
	go func() {
		v, shutdown := q.Get()
		ch <- getResult{v, shutdown}
	}()

	// Add a dirty item.
	q.Add(key)

	select {
	case <-ch:
		t.Fatal("expected Get to block")
	default:
	}

	// Mark that we are done with a, should undirty the item and unblock Get.
	q.Done(key)

	select {
	case result := <-ch:
		assert.Assert(t, !result.shutdown)
		assert.Equal(t, result.v, key)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Get()")
	}

	q.Add(key)
	v, shutdown = q.Get()
	assert.Assert(t, !shutdown)
	assert.Equal(t, v, key)

	// Add again while still working on it
	q.Add(key)
	go func() {
		v, shutdown := q.Get()
		ch <- getResult{v, shutdown}
	}()

	select {
	case <-ch:
		t.Fatal("expected Get to block")
	default:
	}

	// Now mark that we are done with a, should undirty the item and unblock Get.
	q.Done(key)
	select {
	case result := <-ch:
		assert.Assert(t, !result.shutdown)
		assert.Equal(t, result.v, key)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Get()")
	}
	q.Done(key)

	q.Add(key)
	v, shutdown = q.Get()
	assert.Assert(t, !shutdown)
	assert.Equal(t, v, key)

	// Add again while still working on it
	q.Add(key)

	// Should clear the queue
	// Make sure `Get` blocks until shutdown since `Forget` should make it so there are no more items in the queue
	q.Forget(key)

	go func() {
		v, shutdown := q.Get()
		ch <- getResult{v, shutdown}
	}()

	select {
	case <-ch:
		t.Fatal("expected Get to block")
	default:
	}

	q.Close()
	select {
	case result := <-ch:
		assert.Assert(t, result.shutdown)
		assert.Equal(t, result.v, "")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Get()")
	}
}
