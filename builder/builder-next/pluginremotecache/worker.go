package pluginremotecache

import "github.com/docker/docker/builder/builder-next/worker"

type workerStore struct {
	workers map[string]worker.Worker
}
