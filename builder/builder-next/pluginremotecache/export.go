package pluginremotecache

import (
	"context"

	"github.com/docker/docker/pkg/plugingetter"
	"github.com/moby/buildkit/cache/remotecache"
)

const (
	pluginCacheExporterCap = "buildkit.cache.exporter/v1"
)

func LoadCacheExporterPlugins(ctx context.Context, pg plugingetter.PluginGetter) (map[string]remotecache.ResolveCacheExporterFunc, error) {
}
