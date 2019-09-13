package plugin // import "github.com/docker/docker/plugin"

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/remotes"
	"github.com/docker/docker/distribution/xfer"
	"github.com/docker/docker/image"
	"github.com/docker/docker/layer"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/chrootarchive"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/progress"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type blobstore interface {
	New(context.Context, specs.Descriptor) (WriteCommitCloser, error)
	Get(ctx context.Context, dgst digest.Digest) (io.ReadCloser, error)
	Size(ctx context.Context, dgst digest.Digest) (int64, error)
	Walk(context.Context, func(digest.Digest) error) error
	Delete(ctx context.Context, dgst digest.Digest) error
}

type containerdBlobStore struct {
	store content.Store
}

func (s *containerdBlobStore) New(ctx context.Context, desc specs.Descriptor) (WriteCommitCloser, error) {
	w, err := s.store.Writer(ctx, content.WithDescriptor(desc), content.WithRef(remotes.MakeRefKey(ctx, desc)))
	if err != nil {
		return nil, err
	}

	return &containerdWriteWrapper{w, desc}, nil
}

func (s *containerdBlobStore) Get(ctx context.Context, dgst digest.Digest) (io.ReadCloser, error) {
	dec := specs.Descriptor{Digest: dgst}
	ra, err := s.store.ReaderAt(ctx, dec)
	if err != nil {
		return nil, err
	}
	r := content.NewReader(ra)
	return ioutils.NewReadCloserWrapper(r, ra.Close), nil
}

func (s *containerdBlobStore) Size(ctx context.Context, dgst digest.Digest) (int64, error) {
	info, err := s.store.Info(ctx, dgst)
	if err != nil {
		return 0, err
	}

	return info.Size, nil
}

func (s *containerdBlobStore) Walk(ctx context.Context, f func(digest.Digest) error) error {
	return s.store.Walk(ctx, func(info content.Info) error {
		return f(info.Digest)
	})
}

func (s *containerdBlobStore) Delete(ctx context.Context, dgst digest.Digest) error {
	return s.store.Delete(ctx, dgst)
}

type containerdWriteWrapper struct {
	content.Writer
	desc specs.Descriptor
}

func (w *containerdWriteWrapper) Commit(ctx context.Context) (digest.Digest, error) {
	if err := w.Writer.Commit(ctx, w.desc.Size, w.desc.Digest); err != nil {
		return digest.Digest(""), err
	}

	return w.Writer.Digest(), nil
}

type basicBlobStore struct {
	path string
}

func newBasicBlobStore(p string) (*basicBlobStore, error) {
	tmpdir := filepath.Join(p, "tmp")
	if err := os.MkdirAll(tmpdir, 0700); err != nil {
		return nil, errors.Wrapf(err, "failed to mkdir %v", p)
	}
	return &basicBlobStore{path: p}, nil
}

func (b *basicBlobStore) New(ctx context.Context, _ specs.Descriptor) (WriteCommitCloser, error) {
	f, err := ioutil.TempFile(filepath.Join(b.path, "tmp"), ".insertion")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp file")
	}
	return newInsertion(f), nil
}

func (b *basicBlobStore) Get(ctx context.Context, dgst digest.Digest) (io.ReadCloser, error) {
	return os.Open(filepath.Join(b.path, string(dgst.Algorithm()), dgst.Hex()))
}

func (b *basicBlobStore) Size(ctx context.Context, dgst digest.Digest) (int64, error) {
	stat, err := os.Stat(filepath.Join(b.path, string(dgst.Algorithm()), dgst.Hex()))
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (b *basicBlobStore) Walk(ctx context.Context, f func(digest.Digest) error) error {
	for _, alg := range []string{string(digest.Canonical)} {
		items, err := ioutil.ReadDir(filepath.Join(b.path, alg))
		if err != nil {
			continue
		}
		for _, fi := range items {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if err := f(digest.Digest(alg + ":" + fi.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func (b *basicBlobStore) Delete(ctx context.Context, dgst digest.Digest) error {
	p := filepath.Join(b.path, string(dgst.Algorithm()), dgst.Encoded())
	return os.RemoveAll(p)
}

func (b *basicBlobStore) gc(whitelist map[digest.Digest]struct{}) {
	for _, alg := range []string{string(digest.Canonical)} {
		items, err := ioutil.ReadDir(filepath.Join(b.path, alg))
		if err != nil {
			continue
		}
		for _, fi := range items {
			if _, exists := whitelist[digest.Digest(alg+":"+fi.Name())]; !exists {
				p := filepath.Join(b.path, alg, fi.Name())
				err := os.RemoveAll(p)
				logrus.Debugf("cleaned up blob %v: %v", p, err)
			}
		}
	}

}

// WriteCommitCloser defines object that can be committed to blobstore.
type WriteCommitCloser interface {
	io.WriteCloser
	Commit(ctx context.Context) (digest.Digest, error)
}

type insertion struct {
	io.Writer
	f        *os.File
	digester digest.Digester
	closed   bool
}

func newInsertion(tempFile *os.File) *insertion {
	digester := digest.Canonical.Digester()
	return &insertion{f: tempFile, digester: digester, Writer: io.MultiWriter(tempFile, digester.Hash())}
}

func (i *insertion) Commit(ctx context.Context) (digest.Digest, error) {
	p := i.f.Name()
	d := filepath.Join(filepath.Join(p, "../../"))
	i.f.Sync()
	defer os.RemoveAll(p)
	if err := i.f.Close(); err != nil {
		return "", err
	}
	i.closed = true
	dgst := i.digester.Digest()
	if err := os.MkdirAll(filepath.Join(d, string(dgst.Algorithm())), 0700); err != nil {
		return "", errors.Wrapf(err, "failed to mkdir %v", d)
	}
	if err := os.Rename(p, filepath.Join(d, string(dgst.Algorithm()), dgst.Hex())); err != nil {
		return "", errors.Wrapf(err, "failed to rename %v", p)
	}
	return dgst, nil
}

func (i *insertion) Close() error {
	if i.closed {
		return nil
	}
	defer os.RemoveAll(i.f.Name())
	return i.f.Close()
}

type downloadManager struct {
	blobStore    blobstore
	tmpDir       string
	blobs        []digest.Digest
	configDigest digest.Digest
}

func (dm *downloadManager) Download(ctx context.Context, initialRootFS image.RootFS, os string, layers []xfer.DownloadDescriptor, progressOutput progress.Output) (image.RootFS, func(), error) {
	for _, l := range layers {
		b, err := dm.blobStore.New(ctx, specs.Descriptor{})
		if err != nil {
			return initialRootFS, nil, err
		}
		defer b.Close()
		rc, _, err := l.Download(ctx, progressOutput)
		if err != nil {
			return initialRootFS, nil, errors.Wrap(err, "failed to download")
		}
		defer rc.Close()
		r := io.TeeReader(rc, b)
		inflatedLayerData, err := archive.DecompressStream(r)
		if err != nil {
			return initialRootFS, nil, err
		}
		defer inflatedLayerData.Close()
		digester := digest.Canonical.Digester()
		if _, err := chrootarchive.ApplyLayer(dm.tmpDir, io.TeeReader(inflatedLayerData, digester.Hash())); err != nil {
			return initialRootFS, nil, err
		}
		initialRootFS.Append(layer.DiffID(digester.Digest()))
		d, err := b.Commit(ctx)
		if err != nil {
			return initialRootFS, nil, err
		}
		dm.blobs = append(dm.blobs, d)
	}
	return initialRootFS, nil, nil
}

func (dm *downloadManager) Put(dt []byte) (digest.Digest, error) {
	ctx := context.TODO()
	b, err := dm.blobStore.New(ctx, specs.Descriptor{})
	if err != nil {
		return "", err
	}
	defer b.Close()
	n, err := b.Write(dt)
	if err != nil {
		return "", err
	}
	if n != len(dt) {
		return "", io.ErrShortWrite
	}
	d, err := b.Commit(ctx)
	dm.configDigest = d
	return d, err
}

func (dm *downloadManager) Get(d digest.Digest) ([]byte, error) {
	return nil, fmt.Errorf("digest not found")
}
func (dm *downloadManager) RootFSFromConfig(c []byte) (*image.RootFS, error) {
	return configToRootFS(c)
}
func (dm *downloadManager) PlatformFromConfig(c []byte) (*specs.Platform, error) {
	// TODO: LCOW/Plugins. This will need revisiting. For now use the runtime OS
	return &specs.Platform{OS: runtime.GOOS}, nil
}
