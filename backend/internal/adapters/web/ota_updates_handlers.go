package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/httpx"
)

// OTAUpdatesHandler serves the mobile app's over-the-air updates over the
// Expo Updates protocol (https://docs.expo.dev/technical-specs/expo-updates-1/),
// so shipping new JS never goes through EAS. Both routes are public: an
// app checks for updates before anyone signs in. Every field is optional —
// a zero OTAUpdatesHandler (no updates directory configured) answers 404.
type OTAUpdatesHandler struct {
	resolve *services.ResolveUpdateUseCase
	store   ports.UpdateStore
	signer  ports.ManifestSigner
	// publicURL is the absolute origin asset URLs are built on (e.g.
	// "https://budget.example.com"). Empty means "derive it from the
	// request", which is right unless a proxy rewrites Host.
	publicURL string
}

func NewOTAUpdatesHandler(resolve *services.ResolveUpdateUseCase, store ports.UpdateStore, signer ports.ManifestSigner, publicURL string) *OTAUpdatesHandler {
	return &OTAUpdatesHandler{resolve: resolve, store: store, signer: signer, publicURL: strings.TrimRight(publicURL, "/")}
}

type otaAssetJSON struct {
	Hash          string `json:"hash"`
	Key           string `json:"key"`
	ContentType   string `json:"contentType"`
	FileExtension string `json:"fileExtension"`
	URL           string `json:"url"`
}

type otaManifestJSON struct {
	ID             string            `json:"id"`
	CreatedAt      string            `json:"createdAt"`
	RuntimeVersion string            `json:"runtimeVersion"`
	LaunchAsset    otaAssetJSON      `json:"launchAsset"`
	Assets         []otaAssetJSON    `json:"assets"`
	Metadata       map[string]string `json:"metadata"`
	Extra          map[string]any    `json:"extra"`
}

type otaDirectiveJSON struct {
	Type       string         `json:"type"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// Manifest answers GET /api/updates/manifest.
func (h *OTAUpdatesHandler) Manifest(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.resolve == nil {
		httpx.WriteError(w, httpx.NotFound("over-the-air updates are not enabled on this server"))
		return
	}

	protocolHeaders := r.Header.Values("expo-protocol-version")
	if len(protocolHeaders) > 1 {
		httpx.WriteError(w, httpx.BadRequest("unsupported protocol version, expected 0 or 1"))
		return
	}
	protocolVersion := 0
	if len(protocolHeaders) == 1 {
		v, err := strconv.Atoi(protocolHeaders[0])
		if err != nil || v < 0 || v > 1 {
			httpx.WriteError(w, httpx.BadRequest("unsupported protocol version, expected 0 or 1"))
			return
		}
		protocolVersion = v
	}

	req := services.UpdateRequest{
		ProtocolVersion:  protocolVersion,
		Platform:         headerOrQuery(r, "expo-platform", "platform"),
		RuntimeVersion:   headerOrQuery(r, "expo-runtime-version", "runtime-version"),
		CurrentUpdateID:  r.Header.Get("expo-current-update-id"),
		EmbeddedUpdateID: r.Header.Get("expo-embedded-update-id"),
	}

	expectsSignature := r.Header.Get("expo-expect-signature") != ""
	if expectsSignature && h.signer == nil {
		httpx.WriteError(w, httpx.BadRequest("code signing requested but no signing key is configured on this server"))
		return
	}

	decision, err := h.resolve.Resolve(r.Context(), req)
	switch {
	case errors.Is(err, services.ErrUnsupportedPlatform), errors.Is(err, services.ErrMissingRuntime), errors.Is(err, ports.ErrInvalidUpdatePath):
		httpx.WriteError(w, httpx.BadRequest(err.Error()))
		return
	case errors.Is(err, services.ErrNoUpdateForRuntime):
		httpx.WriteError(w, httpx.NotFound(err.Error()))
		return
	case err != nil:
		log.Printf("ota: resolve update: %v", err)
		httpx.WriteError(w, err)
		return
	}

	parts := multipartBody{}
	switch decision.Kind {
	case services.UpdateDecisionUpdate:
		manifest := h.manifestFor(r, decision.Update)
		body, err := json.Marshal(manifest)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		if err := parts.add("manifest", body, h.signatureFor(body, expectsSignature)); err != nil {
			httpx.WriteError(w, err)
			return
		}
		extensions, _ := json.Marshal(map[string]any{"assetRequestHeaders": map[string]any{}})
		if err := parts.add("extensions", extensions, ""); err != nil {
			httpx.WriteError(w, err)
			return
		}
	case services.UpdateDecisionRollBack:
		body, _ := json.Marshal(otaDirectiveJSON{
			Type:       "rollBackToEmbedded",
			Parameters: map[string]any{"commitTime": decision.Update.CreatedAt.Format(time.RFC3339Nano)},
		})
		if err := parts.add("directive", body, h.signatureFor(body, expectsSignature)); err != nil {
			httpx.WriteError(w, err)
			return
		}
	case services.UpdateDecisionNoUpdate:
		body, _ := json.Marshal(otaDirectiveJSON{Type: "noUpdateAvailable"})
		if err := parts.add("directive", body, h.signatureFor(body, expectsSignature)); err != nil {
			httpx.WriteError(w, err)
			return
		}
	}

	if err := parts.close(); err != nil {
		httpx.WriteError(w, err)
		return
	}
	w.Header().Set("expo-protocol-version", strconv.Itoa(protocolVersion))
	w.Header().Set("expo-sfv-version", "0")
	w.Header().Set("cache-control", "private, max-age=0")
	w.Header().Set("content-type", "multipart/mixed; boundary="+parts.writer.Boundary())
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(parts.buf.Bytes())
}

// Asset answers GET /api/updates/assets/{runtimeVersion}/{updateName}/{path...}.
// Published files never change, so they're cacheable forever.
func (h *OTAUpdatesHandler) Asset(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.store == nil {
		http.NotFound(w, r)
		return
	}
	assetPath := r.PathValue("path")
	file, modTime, err := h.store.OpenAsset(r.Context(), r.PathValue("runtimeVersion"), r.PathValue("updateName"), assetPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, ports.ErrInvalidUpdatePath) {
			log.Printf("ota: open asset %q: %v", assetPath, err)
		}
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	ext := strings.TrimPrefix(path.Ext(assetPath), ".")
	if ext == "" {
		// Asset files are stored under their hash with no extension; the
		// client learns the real type from the manifest and asks for it.
		ext = r.URL.Query().Get("ext")
	}
	w.Header().Set("content-type", httpx.ContentTypeForExtension(ext))
	w.Header().Set("cache-control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, "", modTime, file)
}

func (h *OTAUpdatesHandler) manifestFor(r *http.Request, u *entities.OTAUpdate) otaManifestJSON {
	base := h.publicURL
	if base == "" {
		base = requestOrigin(r)
	}
	assetURL := func(a entities.OTAAsset) otaAssetJSON {
		segments := strings.Split(a.Path, "/")
		for i, s := range segments {
			segments[i] = url.PathEscape(s)
		}
		link := fmt.Sprintf("%s/api/updates/assets/%s/%s/%s?ext=%s",
			base, url.PathEscape(u.RuntimeVersion), url.PathEscape(u.Name), strings.Join(segments, "/"),
			url.QueryEscape(strings.TrimPrefix(a.FileExtension, ".")))
		return otaAssetJSON{Hash: a.Hash, Key: a.Key, ContentType: a.ContentType, FileExtension: a.FileExtension, URL: link}
	}

	assets := make([]otaAssetJSON, 0, len(u.Assets))
	for _, a := range u.Assets {
		assets = append(assets, assetURL(a))
	}
	extra := map[string]any{}
	if len(u.ExpoConfig) > 0 {
		extra["expoClient"] = json.RawMessage(u.ExpoConfig)
	}
	return otaManifestJSON{
		ID:             u.ID,
		CreatedAt:      u.CreatedAt.Format(time.RFC3339Nano),
		RuntimeVersion: u.RuntimeVersion,
		LaunchAsset:    assetURL(u.LaunchAsset),
		Assets:         assets,
		Metadata:       map[string]string{},
		Extra:          extra,
	}
}

// signatureFor returns the expo-signature header value (a Structured Field
// dictionary) for body, or "" when the client didn't ask for one.
func (h *OTAUpdatesHandler) signatureFor(body []byte, expected bool) string {
	if !expected || h.signer == nil {
		return ""
	}
	sig, err := h.signer.Sign(body)
	if err != nil {
		log.Printf("ota: sign manifest: %v", err)
		return ""
	}
	return fmt.Sprintf(`sig="%s", keyid="%s"`, sig, h.signer.KeyID())
}

type multipartBody struct {
	buf    bytes.Buffer
	writer *multipart.Writer
}

func (m *multipartBody) add(name string, body []byte, signature string) error {
	if m.writer == nil {
		m.writer = multipart.NewWriter(&m.buf)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"`, name))
	header.Set("Content-Type", "application/json; charset=utf-8")
	if signature != "" {
		header.Set("expo-signature", signature)
	}
	part, err := m.writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = part.Write(body)
	return err
}

func (m *multipartBody) close() error {
	if m.writer == nil {
		m.writer = multipart.NewWriter(&m.buf)
	}
	return m.writer.Close()
}

func headerOrQuery(r *http.Request, header, query string) string {
	if v := r.Header.Get(header); v != "" {
		return v
	}
	return r.URL.Query().Get(query)
}

// requestOrigin rebuilds "scheme://host" as the client saw it, honouring a
// reverse proxy's X-Forwarded-Proto/Host.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p == "https" || p == "http" {
		scheme = p
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = fh
	}
	return scheme + "://" + host
}
