package otaupdates

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const testBucket = "ota"

// fakeS3 starts an in-memory S3 server and returns a Store on it (under
// prefix) plus a function that uploads one object below that prefix.
func fakeS3(t *testing.T, prefix string) (*Store, func(key, content string)) {
	t.Helper()
	server := httptest.NewServer(gofakes3.New(s3mem.New()).Server())
	t.Cleanup(server.Close)

	cfg := S3Config{
		Endpoint:        server.URL,
		Region:          "garage",
		Bucket:          testBucket,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		Prefix:          prefix,
	}
	client, err := minio.New(strings.TrimPrefix(server.URL, "http://"), &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Region:       cfg.Region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.MakeBucket(context.Background(), testBucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
		t.Fatal(err)
	}

	store, err := NewS3Store(cfg)
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	put := func(key, content string) {
		t.Helper()
		full := strings.TrimSuffix(prefix, "/")
		if full != "" {
			full += "/"
		}
		_, err := client.PutObject(context.Background(), testBucket, full+key, strings.NewReader(content), int64(len(content)), minio.PutObjectOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}
	return store, put
}

func publishS3Fixture(put func(key, content string), runtimeVersion, name string) {
	dir := runtimeVersion + "/" + name + "/"
	put(dir+"expoConfig.json", `{"name":"Lagune"}`)
	put(dir+"_expo/static/js/ios/entry-abc.hbc", "bundle-"+name)
	put(dir+"assets/f00", "png-bytes")
	put(dir+"metadata.json", fixtureMetadata)
}

func TestS3Store_Latest(t *testing.T) {
	store, put := fakeS3(t, "budget/")
	publishS3Fixture(put, "1.0.0", "1700000000")
	publishS3Fixture(put, "1.0.0", "1800000000")
	publishS3Fixture(put, "1.0.0", "1750000000")
	publishS3Fixture(put, "1.0.10", "1900000000") // shares the "1.0.1" key prefix
	put("1.0.0/not-a-timestamp/metadata.json", fixtureMetadata)

	update, err := store.Latest(context.Background(), "1.0.0", "ios")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if update == nil || update.Name != "1800000000" || update.IsRollback {
		t.Fatalf("got %+v, want update 1800000000", update)
	}
	if update.LaunchAsset.Path != "_expo/static/js/ios/entry-abc.hbc" || len(update.Assets) != 1 {
		t.Errorf("assets = %+v / %+v", update.LaunchAsset, update.Assets)
	}
	if string(update.ExpoConfig) != `{"name":"Lagune"}` {
		t.Errorf("ExpoConfig = %s", update.ExpoConfig)
	}

	if u, err := store.Latest(context.Background(), "1.0.1", "ios"); err != nil || u != nil {
		t.Errorf("runtime 1.0.1 must not see 1.0.10's updates: got %+v, %v", u, err)
	}
	if u, err := store.Latest(context.Background(), "1.0.0", "android"); err != nil || u != nil {
		t.Errorf("platform not exported: got %+v, %v; want nil, nil", u, err)
	}
}

func TestS3Store_Latest_SkipsIncompleteUpload(t *testing.T) {
	store, put := fakeS3(t, "")
	publishS3Fixture(put, "1.0.0", "1700000000")
	put("1.0.0/1800000000/assets/f00", "png-bytes") // metadata.json not uploaded yet

	update, err := store.Latest(context.Background(), "1.0.0", "ios")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if update == nil || update.Name != "1700000000" {
		t.Fatalf("got %+v, want the previous complete update 1700000000", update)
	}
}

func TestS3Store_Latest_Rollback(t *testing.T) {
	store, put := fakeS3(t, "")
	publishS3Fixture(put, "1.0.0", "1700000000")
	// gofakes3 rejects empty bodies (MissingContentLength); real S3 and
	// Garage accept them, and only the marker's presence matters.
	put("1.0.0/1800000000/rollback", "rollback")

	update, err := store.Latest(context.Background(), "1.0.0", "ios")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if update == nil || !update.IsRollback || update.ID != "rollback-1800000000" {
		t.Fatalf("got %+v, want a rollback", update)
	}
}

func TestS3Store_OpenAsset(t *testing.T) {
	store, put := fakeS3(t, "budget")
	publishS3Fixture(put, "1.0.0", "1700000000")

	f, modTime, err := store.OpenAsset(context.Background(), "1.0.0", "1700000000", "assets/f00")
	if err != nil {
		t.Fatalf("OpenAsset: %v", err)
	}
	defer f.Close()
	if modTime.IsZero() {
		t.Error("modTime should come from the object")
	}
	// http.ServeContent seeks to the end to size the body, then back.
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil || size != int64(len("png-bytes")) {
		t.Fatalf("Seek end = %d, %v", size, err)
	}
	if _, err := f.Seek(4, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if _, err := io.Copy(&body, f); err != nil || body.String() != "bytes" {
		t.Errorf("body from offset 4 = %q, %v", body.String(), err)
	}

	if _, _, err := store.OpenAsset(context.Background(), "1.0.0", "1700000000", "assets/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing asset err = %v, want fs.ErrNotExist", err)
	}
	if _, _, err := store.OpenAsset(context.Background(), "1.0.0", "1700000000", "../../secret"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("unsafe path err = %v, want ErrInvalidName", err)
	}
}

func TestNewS3Store_ValidatesConfig(t *testing.T) {
	valid := S3Config{Endpoint: "https://s3.example.com", Bucket: "ota", AccessKeyID: "id", SecretAccessKey: "secret"}
	if _, err := NewS3Store(valid); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	for name, mutate := range map[string]func(*S3Config){
		"no scheme": func(c *S3Config) { c.Endpoint = "s3.example.com" },
		"ftp":       func(c *S3Config) { c.Endpoint = "ftp://s3.example.com" },
		"no bucket": func(c *S3Config) { c.Bucket = "" },
		"no key id": func(c *S3Config) { c.AccessKeyID = "" },
		"no secret": func(c *S3Config) { c.SecretAccessKey = "" },
	} {
		cfg := valid
		mutate(&cfg)
		if _, err := NewS3Store(cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
