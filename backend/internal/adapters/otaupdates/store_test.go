package otaupdates

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const fixtureMetadata = `{"version":0,"bundler":"metro","fileMetadata":{
  "ios":{"bundle":"_expo/static/js/ios/entry-abc.hbc","assets":[{"path":"assets/f00","ext":"png"}]}
}}`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func publishFixture(t *testing.T, root, runtimeVersion, name string) string {
	t.Helper()
	dir := filepath.Join(root, runtimeVersion, name)
	writeFile(t, filepath.Join(dir, "metadata.json"), fixtureMetadata)
	writeFile(t, filepath.Join(dir, "expoConfig.json"), `{"name":"Lagune"}`)
	writeFile(t, filepath.Join(dir, "_expo/static/js/ios/entry-abc.hbc"), "bundle-"+name)
	writeFile(t, filepath.Join(dir, "assets/f00"), "png-bytes")
	return dir
}

func TestStore_Latest_PicksNewestPublish(t *testing.T) {
	root := t.TempDir()
	publishFixture(t, root, "1.0.0", "1700000000")
	publishFixture(t, root, "1.0.0", "1800000000")
	publishFixture(t, root, "1.0.0", "1750000000")
	if err := os.MkdirAll(filepath.Join(root, "1.0.0", "not-a-timestamp"), 0o755); err != nil {
		t.Fatal(err)
	}

	update, err := NewStore(root).Latest(context.Background(), "1.0.0", "ios")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if update == nil || update.Name != "1800000000" {
		t.Fatalf("got %+v, want update 1800000000", update)
	}
	if update.IsRollback {
		t.Fatal("unexpected rollback")
	}
	if update.CreatedAt.Unix() != 1800000000 {
		t.Errorf("CreatedAt = %v", update.CreatedAt)
	}
	if len(update.ID) != 36 {
		t.Errorf("ID %q is not UUID-shaped", update.ID)
	}

	sum := sha256.Sum256([]byte("bundle-1800000000"))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); update.LaunchAsset.Hash != want {
		t.Errorf("launch hash = %q, want %q", update.LaunchAsset.Hash, want)
	}
	if update.LaunchAsset.ContentType != "application/javascript" || update.LaunchAsset.FileExtension != ".bundle" {
		t.Errorf("launch asset = %+v", update.LaunchAsset)
	}
	if len(update.Assets) != 1 || update.Assets[0].ContentType != "image/png" || update.Assets[0].FileExtension != ".png" {
		t.Errorf("assets = %+v", update.Assets)
	}
	if string(update.ExpoConfig) != `{"name":"Lagune"}` {
		t.Errorf("ExpoConfig = %s", update.ExpoConfig)
	}
}

func TestStore_Latest_SameExportSameID(t *testing.T) {
	root := t.TempDir()
	publishFixture(t, root, "1.0.0", "1700000000")
	first, _ := NewStore(root).Latest(context.Background(), "1.0.0", "ios")
	publishFixture(t, root, "1.0.0", "1800000000")
	second, _ := NewStore(root).Latest(context.Background(), "1.0.0", "ios")
	if first.ID != second.ID {
		t.Errorf("identical metadata.json should give identical IDs: %s vs %s", first.ID, second.ID)
	}
}

func TestStore_Latest_Rollback(t *testing.T) {
	root := t.TempDir()
	publishFixture(t, root, "1.0.0", "1700000000")
	writeFile(t, filepath.Join(root, "1.0.0", "1800000000", "rollback"), "")

	update, err := NewStore(root).Latest(context.Background(), "1.0.0", "ios")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if update == nil || !update.IsRollback {
		t.Fatalf("got %+v, want a rollback", update)
	}
}

func TestStore_Latest_SkipsUpdateWithoutMetadata(t *testing.T) {
	root := t.TempDir()
	publishFixture(t, root, "1.0.0", "1700000000")
	writeFile(t, filepath.Join(root, "1.0.0", "1800000000", "assets", "f00"), "png-bytes")

	update, err := NewStore(root).Latest(context.Background(), "1.0.0", "ios")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if update == nil || update.Name != "1700000000" {
		t.Fatalf("got %+v, want the previous complete update 1700000000", update)
	}
}

func TestStore_Latest_NothingPublished(t *testing.T) {
	root := t.TempDir()
	publishFixture(t, root, "1.0.0", "1700000000")
	store := NewStore(root)

	if u, err := store.Latest(context.Background(), "2.0.0", "ios"); err != nil || u != nil {
		t.Errorf("unknown runtime: got %+v, %v; want nil, nil", u, err)
	}
	if u, err := store.Latest(context.Background(), "1.0.0", "android"); err != nil || u != nil {
		t.Errorf("platform not exported: got %+v, %v; want nil, nil", u, err)
	}
}

func TestStore_Latest_RejectsUnsafeRuntimeVersion(t *testing.T) {
	for _, rv := range []string{"..", "../etc", "a/b", ".hidden", ""} {
		if _, err := NewStore(t.TempDir()).Latest(context.Background(), rv, "ios"); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Latest(%q) err = %v, want ErrInvalidName", rv, err)
		}
	}
}

func TestStore_OpenAsset(t *testing.T) {
	root := t.TempDir()
	publishFixture(t, root, "1.0.0", "1700000000")
	writeFile(t, filepath.Join(root, "secret.txt"), "nope")
	store := NewStore(root)

	f, _, err := store.OpenAsset(context.Background(), "1.0.0", "1700000000", "assets/f00")
	if err != nil {
		t.Fatalf("OpenAsset: %v", err)
	}
	body, _ := io.ReadAll(f)
	f.Close()
	if string(body) != "png-bytes" {
		t.Errorf("body = %q", body)
	}

	for _, bad := range []string{"../../secret.txt", "/etc/passwd", "assets/../../../secret.txt", "assets", ""} {
		if f, _, err := store.OpenAsset(context.Background(), "1.0.0", "1700000000", bad); err == nil {
			f.Close()
			t.Errorf("OpenAsset(%q) should fail", bad)
		}
	}
	if _, _, err := store.OpenAsset(context.Background(), "..", "1700000000", "assets/f00"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("unsafe runtime version err = %v", err)
	}
}

func TestStore_OpenAsset_RefusesSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	dir := publishFixture(t, root, "1.0.0", "1700000000")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeFile(t, outside, "nope")
	if err := os.Symlink(outside, filepath.Join(dir, "assets", "link")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if f, _, err := NewStore(root).OpenAsset(context.Background(), "1.0.0", "1700000000", "assets/link"); err == nil {
		f.Close()
		t.Fatal("a symlink escaping the update directory must not be served")
	}
}
