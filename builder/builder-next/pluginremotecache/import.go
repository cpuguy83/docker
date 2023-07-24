package pluginremotecache

import (
	"bufio"
	"context"
	"errors"
	"io"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/docker/docker/pkg/plugingetter"
	"github.com/docker/docker/pkg/plugins"
	"github.com/moby/buildkit/cache/remotecache"
	v1 "github.com/moby/buildkit/cache/remotecache/v1"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/solver"
	"github.com/moby/buildkit/worker"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const pluginCacheImporterCap = "buildkit.cache.importer/v1"

type LoadRequest struct {
	Descriptor ocispec.Descriptor
}

type CacheChainResponse struct {
	Config v1.CacheConfig
}

type InitRequest struct {
	WorkerProxy string
}

type InitResponse struct {
	ID string
}

type pluginImporter struct {
	p plugingetter.CompatPlugin
}

type ReaderAtRequest struct {
	Descriptor ocispec.Descriptor
	Offset     int64
	Size       int64
}

func (p *pluginImporter) Resolve(ctx context.Context, desc ocispec.Descriptor, id string, w worker.Worker) (solver.CacheManager, error) {
	var (
		ret  CacheChainResponse
		opts []func(*plugins.RequestOpts)
	)

	if dl, ok := ctx.Deadline(); ok {
		opts = append(opts, plugins.WithRequestTimeout(time.Until(dl)))
	}
	err := p.p.Client().CallWithOptions(pluginCacheImporterCap+".Load", LoadRequest{
		Descriptor: desc,
	}, &ret, opts...)
	if err != nil {
		return nil, err
	}

	allLayers := make(v1.DescriptorProvider, len(ret.Config.Layers))
	for _, l := range ret.Config.Layers {
		dpp, err := p.makeDescriptorProviderPair(l)
		if err != nil {
			return nil, err
		}
		allLayers[l.Blob] = dpp
	}

	cc := v1.NewCacheChains()
	if err := v1.ParseConfig(ret.Config, allLayers, cc); err != nil {
		return nil, err
	}
	return cc, nil
}

type pluginProvider struct {
	desc ocispec.Descriptor
	p    plugingetter.CompatPlugin
}

type pluginReaderAt struct {
	desc   ocispec.Descriptor
	client plugingetter.CompatPlugin
	closed bool
}

func (ra *pluginReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if ra.closed {
		return 0, io.EOF
	}
	rc, err := ra.client.Client().Stream(pluginCacheImporterCap+".ReadAt", ReaderAtRequest{Descriptor: ra.desc, Offset: off, Size: int64(len(p))})
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	br := bufio.NewReader(rc)
	n, err = br.Read(p)
	return n, err
}

func (ra *pluginReaderAt) Close() error {
	ra.closed = true
	return nil
}

func (ra *pluginReaderAt) Size() int64 {
	return ra.desc.Size
}

func (p *pluginProvider) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	// TODO: Stat too make sure the plugin has this descriptor?
	// I think due to the only way we'd get here is if the plugin returned this descriptor, as such it should already be there.
	return &pluginReaderAt{desc: desc, client: p.p}, nil
}

func (p *pluginImporter) makeDescriptorProviderPair(l v1.CacheLayer) (v1.DescriptorProviderPair, error) {
	if l.Annotations == nil {
		return v1.DescriptorProviderPair{}, errors.New("cache layer with missing annotations")
	}
	annotations := map[string]string{}
	if l.Annotations.DiffID == "" {
		return v1.DescriptorProviderPair{}, errors.New("cache layer with missing diffid")
	}
	annotations["containerd.io/uncompressed"] = l.Annotations.DiffID.String()
	if !l.Annotations.CreatedAt.IsZero() {
		txt, err := l.Annotations.CreatedAt.MarshalText()
		if err != nil {
			return v1.DescriptorProviderPair{}, err
		}
		annotations["buildkit/createdat"] = string(txt)
	}

	desc := ocispec.Descriptor{
		MediaType:   l.Annotations.MediaType,
		Digest:      l.Blob,
		Size:        l.Annotations.Size,
		Annotations: annotations,
	}
	return v1.DescriptorProviderPair{
		Descriptor: desc,
		Provider:   &pluginProvider{desc: desc, p: p.p},
	}, nil
}

func LoadCacheImporterPlugins(ctx context.Context, pg plugingetter.PluginGetter) (map[string]remotecache.ResolveCacheImporterFunc, error) {
	ls, err := pg.GetAllByCap(pluginCacheImporterCap)
	if err != nil {
		return nil, err
	}

	importers := make(map[string]remotecache.ResolveCacheImporterFunc, len(ls))
	for _, p := range ls {
		importers[p.Name()] = pluginToImporterFunc(p)
	}
	return importers, nil
}

func pluginToImporterFunc(p plugingetter.CompatPlugin) remotecache.ResolveCacheImporterFunc {
	return func(ctx context.Context, g session.Group, attrs map[string]string) (remotecache.Importer, ocispec.Descriptor, error) {
		return &pluginImporter{p: p}, ocispec.Descriptor{}, nil
	}
}
