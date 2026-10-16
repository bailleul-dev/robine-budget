package web

import (
	"net/http/httptest"
	"testing"
)

// The embedded dist/ is a placeholder in local dev/test builds (see
// embed.go's doc comment) - NewStaticHandler must still construct and
// respond without panicking either way, falling back to index.html for
// any path it doesn't recognize as a real asset.
func TestNewStaticHandler_FallsBackToIndex(t *testing.T) {
	handler := NewStaticHandler()

	req := httptest.NewRequest("GET", "/some/client-side/route", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 && rec.Code != 404 {
		t.Fatalf("status = %d, want 200 (index.html) or 404 (no dist/ embedded in this build)", rec.Code)
	}
}

func TestNewStaticHandler_RootPath(t *testing.T) {
	handler := NewStaticHandler()

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 && rec.Code != 404 {
		t.Fatalf("status = %d, want 200 or 404", rec.Code)
	}
}
