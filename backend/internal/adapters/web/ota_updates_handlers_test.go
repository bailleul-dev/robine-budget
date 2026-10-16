package web

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"bailleul.dev/budget-server/internal/adapters/otaupdates"
	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/domain/services"
)

// publishOTAFixture lays out one `expo export` the way `make ota-publish`
// stores it.
func publishOTAFixture(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, "1.0.0", name)
	files := map[string]string{
		"metadata.json":                     `{"version":0,"bundler":"metro","fileMetadata":{"ios":{"bundle":"_expo/static/js/ios/entry-abc.hbc","assets":[{"path":"assets/f00","ext":"png"}]}}}`,
		"expoConfig.json":                   `{"name":"Lagune"}`,
		"_expo/static/js/ios/entry-abc.hbc": "bundle-bytes",
		"assets/f00":                        "png-bytes",
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newOTATestHandler(t *testing.T, root string, withSigner bool) (*OTAUpdatesHandler, *rsa.PublicKey) {
	t.Helper()
	store := otaupdates.NewStore(root)
	h := NewOTAUpdatesHandler(services.NewResolveUpdateUseCase(store), store, nil, "https://budget.example.com/")
	if !withSigner {
		return h, nil
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	keyPath := filepath.Join(t.TempDir(), "private-key.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	signer, err := security.LoadRSASigner(keyPath, "main")
	if err != nil {
		t.Fatal(err)
	}
	h.signer = signer
	return h, &key.PublicKey
}

type otaPart struct {
	header http.Header
	body   []byte
}

func manifestRequest(headers map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/updates/manifest", nil)
	req.Header.Set("expo-protocol-version", "1")
	req.Header.Set("expo-platform", "ios")
	req.Header.Set("expo-runtime-version", "1.0.0")
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	return req
}

func readOTAParts(t *testing.T, rec *httptest.ResponseRecorder) map[string]otaPart {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(rec.Header().Get("content-type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("content-type = %q, want multipart/mixed", rec.Header().Get("content-type"))
	}
	parts := map[string]otaPart{}
	reader := multipart.NewReader(rec.Body, params["boundary"])
	for {
		p, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		body, _ := io.ReadAll(p)
		parts[p.FormName()] = otaPart{header: http.Header(p.Header), body: body}
	}
	return parts
}

func TestOTAManifest_ServesLatestUpdate(t *testing.T) {
	root := t.TempDir()
	publishOTAFixture(t, root, "1700000000")
	h, _ := newOTATestHandler(t, root, false)

	rec := httptest.NewRecorder()
	h.Manifest(rec, manifestRequest(nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	for header, want := range map[string]string{"expo-protocol-version": "1", "expo-sfv-version": "0", "cache-control": "private, max-age=0"} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	parts := readOTAParts(t, rec)
	if _, ok := parts["extensions"]; !ok {
		t.Error("missing extensions part")
	}
	var manifest otaManifestJSON
	if err := json.Unmarshal(parts["manifest"].body, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.RuntimeVersion != "1.0.0" || manifest.ID == "" || manifest.CreatedAt == "" {
		t.Errorf("manifest = %+v", manifest)
	}
	wantURL := "https://budget.example.com/api/updates/assets/1.0.0/1700000000/_expo/static/js/ios/entry-abc.hbc?ext=bundle"
	if manifest.LaunchAsset.URL != wantURL {
		t.Errorf("launch URL = %q, want %q", manifest.LaunchAsset.URL, wantURL)
	}
	if len(manifest.Assets) != 1 || manifest.Assets[0].ContentType != "image/png" {
		t.Errorf("assets = %+v", manifest.Assets)
	}
	if !strings.Contains(string(parts["manifest"].body), `"expoClient":{"name":"Lagune"}`) {
		t.Errorf("manifest should carry the expo config as extra.expoClient: %s", parts["manifest"].body)
	}
	if parts["manifest"].header.Get("expo-signature") != "" {
		t.Error("no signature expected when the client didn't ask for one")
	}
}

func TestOTAManifest_SignsWhenAsked(t *testing.T) {
	root := t.TempDir()
	publishOTAFixture(t, root, "1700000000")
	h, pub := newOTATestHandler(t, root, true)

	rec := httptest.NewRecorder()
	h.Manifest(rec, manifestRequest(map[string]string{"expo-expect-signature": `sig, keyid="main", alg="rsa-v1_5-sha256"`}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	part := readOTAParts(t, rec)["manifest"]
	m := regexp.MustCompile(`^sig="([^"]+)", keyid="main"$`).FindStringSubmatch(part.header.Get("expo-signature"))
	if m == nil {
		t.Fatalf("expo-signature = %q", part.header.Get("expo-signature"))
	}
	sig, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(part.body)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("signature doesn't verify against the exact manifest bytes: %v", err)
	}
}

func TestOTAManifest_SignatureRequestedWithoutKey(t *testing.T) {
	root := t.TempDir()
	publishOTAFixture(t, root, "1700000000")
	h, _ := newOTATestHandler(t, root, false)

	rec := httptest.NewRecorder()
	h.Manifest(rec, manifestRequest(map[string]string{"expo-expect-signature": `sig, keyid="main"`}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestOTAManifest_NoUpdateAvailableDirective(t *testing.T) {
	root := t.TempDir()
	publishOTAFixture(t, root, "1700000000")
	h, _ := newOTATestHandler(t, root, false)

	// First learn the current update's ID, then ask again as if running it.
	first := httptest.NewRecorder()
	h.Manifest(first, manifestRequest(nil))
	var manifest otaManifestJSON
	_ = json.Unmarshal(readOTAParts(t, first)["manifest"].body, &manifest)

	rec := httptest.NewRecorder()
	h.Manifest(rec, manifestRequest(map[string]string{"expo-current-update-id": manifest.ID}))
	parts := readOTAParts(t, rec)
	if _, ok := parts["manifest"]; ok {
		t.Error("should not resend the manifest the client already runs")
	}
	if !strings.Contains(string(parts["directive"].body), `"noUpdateAvailable"`) {
		t.Errorf("directive = %s", parts["directive"].body)
	}
}

func TestOTAManifest_RollbackDirective(t *testing.T) {
	root := t.TempDir()
	publishOTAFixture(t, root, "1700000000")
	rollbackDir := filepath.Join(root, "1.0.0", "1800000000")
	_ = os.MkdirAll(rollbackDir, 0o755)
	_ = os.WriteFile(filepath.Join(rollbackDir, "rollback"), nil, 0o644)
	h, _ := newOTATestHandler(t, root, false)

	rec := httptest.NewRecorder()
	h.Manifest(rec, manifestRequest(map[string]string{"expo-current-update-id": "some-update", "expo-embedded-update-id": "embedded"}))
	body := string(readOTAParts(t, rec)["directive"].body)
	if !strings.Contains(body, `"rollBackToEmbedded"`) || !strings.Contains(body, `"commitTime"`) {
		t.Errorf("directive = %s", body)
	}
}

func TestOTAManifest_BadRequests(t *testing.T) {
	root := t.TempDir()
	publishOTAFixture(t, root, "1700000000")
	h, _ := newOTATestHandler(t, root, false)

	cases := map[string]struct {
		headers map[string]string
		want    int
	}{
		"web platform":       {map[string]string{"expo-platform": "web"}, http.StatusBadRequest},
		"no runtime version": {map[string]string{"expo-runtime-version": ""}, http.StatusBadRequest},
		"unsafe runtime":     {map[string]string{"expo-runtime-version": "../x"}, http.StatusBadRequest},
		"bad protocol":       {map[string]string{"expo-protocol-version": "7"}, http.StatusBadRequest},
		"v0 unknown runtime": {map[string]string{"expo-protocol-version": "0", "expo-runtime-version": "9.9.9"}, http.StatusNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.Manifest(rec, manifestRequest(tc.headers))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestOTAManifest_DisabledWhenNotConfigured(t *testing.T) {
	var h *OTAUpdatesHandler
	rec := httptest.NewRecorder()
	h.Manifest(rec, manifestRequest(nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestOTAAsset_ServesImmutableFile(t *testing.T) {
	root := t.TempDir()
	publishOTAFixture(t, root, "1700000000")
	h, _ := newOTATestHandler(t, root, false)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/updates/assets/{runtimeVersion}/{updateName}/{path...}", h.Asset)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/updates/assets/1.0.0/1700000000/assets/f00?ext=png", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "png-bytes" {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("content-type"); ct != "image/png" {
		t.Errorf("content-type = %q", ct)
	}
	if cc := rec.Header().Get("cache-control"); !strings.Contains(cc, "immutable") {
		t.Errorf("cache-control = %q", cc)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/updates/assets/1.0.0/1700000000/_expo/static/js/ios/entry-abc.hbc?ext=bundle", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("content-type") != "application/javascript" {
		t.Fatalf("bundle: status = %d, content-type = %q", rec.Code, rec.Header().Get("content-type"))
	}

	for _, target := range []string{
		"/api/updates/assets/1.0.0/1700000000/assets/missing",
		"/api/updates/assets/1.0.0/1700000000/%2e%2e/%2e%2e/%2e%2e/etc/passwd",
		"/api/updates/assets/1.0.0/1700000000/metadata.json/..",
	} {
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("%s should not be served", target)
		}
	}
}
