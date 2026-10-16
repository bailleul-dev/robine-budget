package web

import (
	"net/http/httptest"
	"testing"
)

func TestVersionHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/version", nil)
	rec := httptest.NewRecorder()
	VersionHandler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp versionResponse
	decodeJSONBody(t, rec, &resp)
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}
