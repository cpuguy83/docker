package daemon

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	mounttypes "github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/container"
	"github.com/docker/docker/pkg/stringid"
	volumestore "github.com/docker/docker/volume/store"
)

// mountRegister handles all extra mounts required by a container which need to
// be mounted in the daemon mount namespace.
// mountRegister is used as a global store for mounts required by a daemon.
//
// Do not use this unless the mount must exist in the host mount namespace,
// otherwise use the container runtime to handle the mounts where they can be
// automatically cleaned up.
type mountRegister struct {
	root string

	mu       sync.Mutex
	refIndex map[string]map[string]*referencedMount // map[container id]map[mount id]mount
}

type referencedMount struct {
	path string
	mu   sync.Mutex
	refs int
	err  error
}

func (r *mountRegister) Get(ref, id string, mountFn func(string) error) (string, error) {
	r.mu.Lock()

	mounts, ok := r.refIndex[ref]
	if ok {
		mnt, ok := mounts[id]
		if ok {
			mnt.mu.Lock()
			if mnt.err == nil {
				r.mu.Unlock()
				mnt.refs++
				mnt.mu.Unlock()
				return mnt.path, nil
			}
			mnt.mu.Unlock()
		}
	}

	dst := filepath.Join(r.root, stringid.GenerateNonCryptoID())
	mnt := &referencedMount{
		path: dst,
		refs: 1,
	}

	if r.refIndex == nil {
		r.refIndex = make(map[string]map[string]*referencedMount)
	}

	refEntry := r.refIndex[ref]
	if refEntry == nil {
		refEntry = make(map[string]*referencedMount)
		r.refIndex[ref] = refEntry
	}
	refEntry[id] = mnt

	mnt.mu.Lock()
	r.mu.Unlock()

	err := mountFn(dst)
	mnt.err = err
	mnt.mu.Unlock()

	if err != nil {
		r.mu.Lock()
		delete(r.refIndex[ref], id)
		r.mu.Unlock()
		return "", err
	}

	return dst, nil
}

func (r *mountRegister) Release(ref, id string, unmountFn func(string) error) error {
	r.mu.Lock()

	refEntry := r.refIndex[ref]
	if refEntry == nil {
		r.mu.Unlock()
		return nil
	}

	m := refEntry[id]
	if m == nil {
		r.mu.Unlock()
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.refs--
	if m.refs != 0 {
		r.mu.Unlock()
		return nil
	}

	delete(r.refIndex[ref], id)
	r.mu.Unlock()

	return unmountFn(m.path)
}

func (daemon *Daemon) prepareMountPoints(container *container.Container) error {
	for _, config := range container.MountPoints {
		if err := daemon.lazyInitializeVolume(container.ID, config); err != nil {
			return err
		}
	}
	return nil
}

func (daemon *Daemon) removeMountPoints(container *container.Container, rm bool) error {
	var rmErrors []string
	for _, m := range container.MountPoints {
		if m.Type != mounttypes.TypeVolume || m.Volume == nil {
			continue
		}
		daemon.volumes.Dereference(m.Volume, container.ID)
		if !rm {
			continue
		}

		// Do not remove named mountpoints
		// these are mountpoints specified like `docker run -v <name>:/foo`
		if m.Spec.Source != "" {
			continue
		}

		err := daemon.volumes.Remove(m.Volume)
		// Ignore volume in use errors because having this
		// volume being referenced by other container is
		// not an error, but an implementation detail.
		// This prevents docker from logging "ERROR: Volume in use"
		// where there is another container using the volume.
		if err != nil && !volumestore.IsInUse(err) {
			rmErrors = append(rmErrors, err.Error())
		}
	}

	if len(rmErrors) > 0 {
		return fmt.Errorf("Error removing volumes:\n%v", strings.Join(rmErrors, "\n"))
	}
	return nil
}
