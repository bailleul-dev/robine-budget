// Package otaupdates stores the mobile app's over-the-air updates,
// implementing ports.UpdateStore, either in a local directory (NewStore) or
// in an S3-compatible bucket such as Garage (NewS3Store). Both hold exactly
// what `make ota-publish` writes:
//
//	<root>/<runtimeVersion>/<unix timestamp>/
//	    metadata.json        (from `expo export`)
//	    expoConfig.json      (from `expo config --type public --json`)
//	    _expo/static/js/...  (the Hermes bundles)
//	    assets/...
//
// and, for a rollback published by `make ota-rollback`:
//
//	<root>/<runtimeVersion>/<unix timestamp>/rollback
//
// The newest complete timestamp directory wins: one without metadata.json
// or a rollback marker yet is still being uploaded and is skipped (the
// publish script writes metadata.json last). Published directories are
// treated as immutable, which is what lets asset hashes be cached in memory.
package otaupdates

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/httpx"
)

// ErrInvalidName is returned for a runtime version, update name or asset
// path that isn't a plain, safe path component.
var ErrInvalidName = ports.ErrInvalidUpdatePath

// safeSegment matches runtime versions ("1.0.0") and update directory
// names ("1789638058"): no separators, can't be "." or "..".
var safeSegment = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)

// source is where published files live. Keys are slash-separated paths
// relative to the store root ("1.0.0/1789638058/metadata.json"), always
// validated before reaching a source. Missing files are fs.ErrNotExist.
type source interface {
	// listUpdates returns the names of the update directories directly
	// under runtimeVersion (none, not an error, when it doesn't exist).
	listUpdates(ctx context.Context, runtimeVersion string) ([]string, error)
	readFile(ctx context.Context, key string) ([]byte, error)
	exists(ctx context.Context, key string) (bool, error)
	// open must not serve anything resolving outside the update's own
	// directory.
	open(ctx context.Context, runtimeVersion, updateName, assetPath string) (io.ReadSeekCloser, time.Time, error)
}

type Store struct {
	src source

	mu     sync.Mutex
	hashes map[string]entities.OTAAsset // keyed by source key
}

func newStore(src source) *Store {
	return &Store{src: src, hashes: map[string]entities.OTAAsset{}}
}

type exportMetadata struct {
	FileMetadata map[string]struct {
		Bundle string `json:"bundle"`
		Assets []struct {
			Path string `json:"path"`
			Ext  string `json:"ext"`
		} `json:"assets"`
	} `json:"fileMetadata"`
}

func (s *Store) Latest(ctx context.Context, runtimeVersion, platform string) (*entities.OTAUpdate, error) {
	if !safeSegment.MatchString(runtimeVersion) {
		return nil, ErrInvalidName
	}
	names, err := s.src.listUpdates(ctx, runtimeVersion)
	if err != nil {
		return nil, fmt.Errorf("list updates for runtime %s: %w", runtimeVersion, err)
	}

	type candidate struct {
		name string
		ts   int64
	}
	var candidates []candidate
	for _, name := range names {
		ts, err := strconv.ParseInt(name, 10, 64)
		if err != nil {
			continue // not a publish directory
		}
		candidates = append(candidates, candidate{name, ts})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ts > candidates[j].ts })

	for _, c := range candidates {
		update, err := s.load(ctx, runtimeVersion, platform, c.name, c.ts)
		if errors.Is(err, errIncomplete) {
			continue
		}
		return update, err
	}
	return nil, nil
}

// errIncomplete marks an update directory still being uploaded.
var errIncomplete = errors.New("incomplete update")

func (s *Store) load(ctx context.Context, runtimeVersion, platform, name string, ts int64) (*entities.OTAUpdate, error) {
	updateDir := runtimeVersion + "/" + name
	update := &entities.OTAUpdate{
		Name:           name,
		RuntimeVersion: runtimeVersion,
		Platform:       platform,
		CreatedAt:      timestampToTime(ts),
	}

	isRollback, err := s.src.exists(ctx, updateDir+"/rollback")
	if err != nil {
		return nil, fmt.Errorf("check rollback marker of update %s: %w", updateDir, err)
	}
	if isRollback {
		update.IsRollback = true
		update.ID = "rollback-" + name
		return update, nil
	}

	rawMetadata, err := s.src.readFile(ctx, updateDir+"/metadata.json")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errIncomplete
	}
	if err != nil {
		return nil, fmt.Errorf("read metadata of update %s: %w", updateDir, err)
	}
	var metadata exportMetadata
	if err := json.Unmarshal(rawMetadata, &metadata); err != nil {
		return nil, fmt.Errorf("parse metadata of update %s: %w", updateDir, err)
	}
	platformMetadata, ok := metadata.FileMetadata[platform]
	if !ok || platformMetadata.Bundle == "" {
		// Published without this platform (e.g. `-p ios` only).
		return nil, nil
	}

	sum := sha256.Sum256(rawMetadata)
	update.ID = hashToUUID(hex.EncodeToString(sum[:]))

	launch, err := s.describe(ctx, updateDir, platformMetadata.Bundle, "bundle")
	if err != nil {
		return nil, err
	}
	launch.ContentType = "application/javascript"
	update.LaunchAsset = launch

	for _, a := range platformMetadata.Assets {
		asset, err := s.describe(ctx, updateDir, a.Path, a.Ext)
		if err != nil {
			return nil, err
		}
		update.Assets = append(update.Assets, asset)
	}

	if cfg, err := s.src.readFile(ctx, updateDir+"/expoConfig.json"); err == nil && json.Valid(cfg) {
		update.ExpoConfig = cfg
	}
	return update, nil
}

// describe hashes one asset file (memoized: published files never change).
func (s *Store) describe(ctx context.Context, updateDir, relPath, ext string) (entities.OTAAsset, error) {
	if !isSafeRelPath(relPath) {
		return entities.OTAAsset{}, ErrInvalidName
	}
	key := updateDir + "/" + relPath

	s.mu.Lock()
	cached, ok := s.hashes[key]
	s.mu.Unlock()
	if ok {
		return cached, nil
	}

	data, err := s.src.readFile(ctx, key)
	if err != nil {
		return entities.OTAAsset{}, fmt.Errorf("read asset %s: %w", relPath, err)
	}
	sha := sha256.Sum256(data)
	md := md5.Sum(data)
	asset := entities.OTAAsset{
		Path:          relPath,
		Hash:          base64.RawURLEncoding.EncodeToString(sha[:]),
		Key:           hex.EncodeToString(md[:]),
		ContentType:   httpx.ContentTypeForExtension(ext),
		FileExtension: "." + ext,
	}

	s.mu.Lock()
	s.hashes[key] = asset
	s.mu.Unlock()
	return asset, nil
}

func (s *Store) OpenAsset(ctx context.Context, runtimeVersion, updateName, assetPath string) (io.ReadSeekCloser, time.Time, error) {
	if !safeSegment.MatchString(runtimeVersion) || !safeSegment.MatchString(updateName) || !isSafeRelPath(assetPath) {
		return nil, time.Time{}, ErrInvalidName
	}
	return s.src.open(ctx, runtimeVersion, updateName, assetPath)
}

func isSafeRelPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	clean := path.Clean(p)
	return clean == p && clean != "." && !strings.HasPrefix(clean, "../") && clean != ".."
}

// timestampToTime accepts unix seconds or milliseconds.
func timestampToTime(ts int64) time.Time {
	if ts > 1e12 {
		return time.UnixMilli(ts).UTC()
	}
	return time.Unix(ts, 0).UTC()
}

// hashToUUID formats the first 32 hex chars of a hash as a UUID, the same
// way Expo's reference server derives update IDs.
func hashToUUID(h string) string {
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}
