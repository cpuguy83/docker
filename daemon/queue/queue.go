package queue

import (
	"sync"

	"github.com/docker/go-metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Unique is a queue that only allows one item of a particular value to be in the queue at a time.
// Items that are currently processing can be added, but will not actually be available to get from the queue until after processing has completed.
type Unique struct {
	mu      sync.Mutex
	cond    *sync.Cond
	dirty   map[string]struct{}
	working map[string]struct{}
	items   []string
	closed  bool

	processed metrics.Counter
	aborted   metrics.Counter
}

func (q *Unique) setup() {
	if q.cond == nil {
		q.cond = sync.NewCond(&q.mu)
	}
	if q.dirty == nil {
		q.dirty = make(map[string]struct{})
	}
	if q.working == nil {
		q.working = make(map[string]struct{})
	}
	if q.processed == nil {
		q.processed = nopCounter{}
	}
	if q.aborted == nil {
		q.aborted = nopCounter{}
	}
}

// Push adds an item to the queue.
// If the item is already in the queue, it will not be added again.
func (q *Unique) Add(item string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.setup()

	q.dirty[item] = struct{}{}
	if _, ok := q.working[item]; ok {
		return
	}

	q.items = append(q.items, item)
	q.cond.Signal()
}

// Get returns the next item in the queue.
// If no items in in queue, it will block until an item is pushed.
// If the queue is closed, it will return an empty string.
func (q *Unique) Get() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.setup()

	for len(q.items) == 0 && !q.closed {
		q.cond.Wait()
	}

	if q.closed {
		return "", true
	}

	item := q.items[0]
	q.items = q.items[1:]
	q.working[item] = struct{}{}
	delete(q.dirty, item)

	return item, false
}

// Done marks items as done processing.
// If the item was added again during processing (aka "dirty"), it will be added to the end of the queue again.
func (q *Unique) Done(item string) {
	q.mu.Lock()

	delete(q.working, item)
	if _, ok := q.dirty[item]; ok {
		q.items = append(q.items, item)
		q.cond.Signal()
	} else if len(q.working) == 0 {
		q.cond.Signal()
	}
	q.mu.Unlock()
	q.processed.Inc()
}

// Forget removes an item from the queue.
// This should generally be used for totally aborting a working item (such as on a fatal error).
// However, it can also be used to remove an item still in the queue.
func (q *Unique) Forget(item string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.dirty[item]; ok {
		delete(q.dirty, item)
		for i, key := range q.items {
			if item == key {
				q.items = append(q.items[:i], q.items[i+1:]...)
				break
			}
		}
	}
	delete(q.working, item)
	q.aborted.Inc()
}

// Close closes out the queue.
// Any workers calling Get will receive an empty string after this is called.
func (q *Unique) Close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.items = nil
	q.dirty = nil
	q.working = nil
	q.mu.Unlock()
}

func New(name string) *Unique {
	ns := metrics.NewNamespace("queue", name, nil)
	q := &Unique{
		dirty:     map[string]struct{}{},
		working:   map[string]struct{}{},
		processed: ns.NewCounter("processed", "Number of items processed"),
		aborted:   ns.NewCounter("aborted", "Number of items aborted"),
	}
	sc := &stateCollector{
		q: q,
		desc: ns.NewDesc(
			"queue_depths",
			`current length the queue broken down into waiting to be processed (queued), processing (working), and items that need to be reprocessed after current in-progress work is completed (dirty)`,
			metrics.Unit("items"),
			"depth",
		),
	}

	q.cond = sync.NewCond(&q.mu)
	ns.Add(sc)

	return q
}

type stateCollector struct {
	q    *Unique
	desc *prometheus.Desc
}

func (c *stateCollector) Collect(ch chan<- prometheus.Metric) {
	queued, dirty, working := c.q.depths()

	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(queued), "queued")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(dirty), "dirty")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(working), "working")
}

func (q *Unique) depths() (queued, dirty, working int) {
	q.mu.Lock()
	queued, dirty, working = len(q.items), len(q.dirty), len(q.working)
	q.mu.Unlock()
	return
}

func (c *stateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

type nopCounter struct{}

func (nopCounter) Inc(vs ...float64) {}
