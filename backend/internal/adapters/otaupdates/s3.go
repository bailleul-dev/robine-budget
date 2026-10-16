package otaupdates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config points at a bucket on an S3-compatible server. Garage (see
// ../silo01/roles/garage) only supports path-style addressing, which is
// what NewS3Store uses.
type S3Config struct {
	// Endpoint is the server's base URL, e.g. "http://garage.garage.svc:3900".
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	// Prefix is prepended to every key, so updates can share a bucket.
	// Optional.
	Prefix string
}

// NewS3Store serves updates from an S3 bucket, which `make ota-publish
// OTA_DEST=s3://bucket/prefix` fills. Uploads aren't atomic, so the
// publish script writes metadata.json (or the rollback marker) last: until
// then, the Store skips that update.
func NewS3Store(cfg S3Config) (*Store, error) {
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("S3 endpoint %q must be an http(s) URL", cfg.Endpoint)
	}
	if cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, errors.New("S3 bucket, access key ID and secret access key are required")
	}
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure:       endpoint.Scheme == "https",
		Region:       cfg.Region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	prefix := strings.Trim(cfg.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return newStore(s3Source{client: client, bucket: cfg.Bucket, prefix: prefix}), nil
}

type s3Source struct {
	client *minio.Client
	bucket string
	prefix string
}

func (s s3Source) listUpdates(ctx context.Context, runtimeVersion string) ([]string, error) {
	dir := s.prefix + runtimeVersion + "/"
	var names []string
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: dir}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		// Non-recursive listing: sub-"directories" come back as keys
		// ending in "/".
		if name, ok := strings.CutSuffix(strings.TrimPrefix(obj.Key, dir), "/"); ok && name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func (s s3Source) readFile(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.prefix+key, minio.GetObjectOptions{})
	if err != nil {
		return nil, notExist(err)
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, notExist(err)
	}
	return data, nil
}

func (s s3Source) exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, s.prefix+key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	if err := notExist(err); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// open needs no containment check beyond the Store's lexical ones: object
// keys are opaque strings, with no symlinks or ".." resolution.
func (s s3Source) open(ctx context.Context, runtimeVersion, updateName, assetPath string) (io.ReadSeekCloser, time.Time, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.prefix+runtimeVersion+"/"+updateName+"/"+assetPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, time.Time{}, notExist(err)
	}
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, time.Time{}, notExist(err)
	}
	return obj, info.LastModified, nil
}

// notExist maps S3's "no such key" answers to fs.ErrNotExist.
func notExist(err error) error {
	resp := minio.ToErrorResponse(err)
	if resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %v", fs.ErrNotExist, err)
	}
	return err
}
