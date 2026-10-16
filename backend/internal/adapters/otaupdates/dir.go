package otaupdates

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// NewStore serves updates from a local directory (OTA_UPDATES_DIR), which
// `make ota-publish OTA_DEST=<dir or user@host:dir>` fills. Each update is
// staged under a non-numeric name and renamed into place, so a half-copied
// one is never visible.
func NewStore(root string) *Store {
	return newStore(dirSource{root: root})
}

type dirSource struct {
	root string
}

func (d dirSource) listUpdates(_ context.Context, runtimeVersion string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(d.root, runtimeVersion))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func (d dirSource) readFile(_ context.Context, key string) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.root, filepath.FromSlash(key)))
}

func (d dirSource) exists(_ context.Context, key string) (bool, error) {
	_, err := os.Stat(filepath.Join(d.root, filepath.FromSlash(key)))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (d dirSource) open(_ context.Context, runtimeVersion, updateName, assetPath string) (io.ReadSeekCloser, time.Time, error) {
	// os.Root refuses anything (including symlinks) resolving outside the
	// update's own directory, on top of the Store's lexical checks.
	root, err := os.OpenRoot(filepath.Join(d.root, runtimeVersion, updateName))
	if err != nil {
		return nil, time.Time{}, err
	}
	defer root.Close()

	f, err := root.Open(filepath.FromSlash(assetPath))
	if err != nil {
		return nil, time.Time{}, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, time.Time{}, err
	}
	if info.IsDir() {
		f.Close()
		return nil, time.Time{}, fs.ErrNotExist
	}
	return f, info.ModTime(), nil
}
