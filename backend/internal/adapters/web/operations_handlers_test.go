package web

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

func TestOperationCursor_RoundTrip(t *testing.T) {
	original := entities.OperationPageCursor{
		Date:      civil.New(2026, time.March, 5),
		CreatedAt: time.Date(2026, 3, 5, 14, 30, 0, 123456789, time.UTC),
		ID:        uuid.New(),
	}
	encoded := encodeOperationCursor(original)
	if encoded == "" {
		t.Fatal("encodeOperationCursor returned an empty string")
	}

	decoded, err := decodeOperationCursor(encoded)
	if err != nil {
		t.Fatalf("decodeOperationCursor: %v", err)
	}
	if decoded == nil {
		t.Fatal("decodeOperationCursor returned nil for a valid cursor")
	}
	if !decoded.Date.Equal(original.Date) {
		t.Errorf("Date = %s, want %s", decoded.Date, original.Date)
	}
	if !decoded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt = %s, want %s", decoded.CreatedAt, original.CreatedAt)
	}
	if decoded.ID != original.ID {
		t.Errorf("ID = %s, want %s", decoded.ID, original.ID)
	}
}

func TestDecodeOperationCursor_Empty(t *testing.T) {
	decoded, err := decodeOperationCursor("")
	if err != nil {
		t.Fatalf("decodeOperationCursor(\"\"): %v", err)
	}
	if decoded != nil {
		t.Fatalf("got %v, want nil for an empty cursor (start from the first page)", decoded)
	}
}

func TestDecodeOperationCursor_Invalid(t *testing.T) {
	cases := []string{
		"not-valid-base64!!!",
		"aGVsbG8=",                                // valid base64 ("hello"), but not 3 pipe-delimited fields
		"MjAyNi0wMy0wNXxub3QtYS10aW1lc3RhbXB8bm9wZQ==", // valid shape, but the date/time/uuid parts are garbage
	}
	for _, c := range cases {
		if _, err := decodeOperationCursor(c); err == nil {
			t.Errorf("decodeOperationCursor(%q): expected an error, got nil", c)
		}
	}
}

func TestParsePageLimit_Default(t *testing.T) {
	req := httptest.NewRequest("GET", "/operations", nil)
	got, err := parsePageLimit(req, 100, 500)
	if err != nil {
		t.Fatalf("parsePageLimit: %v", err)
	}
	if got != 100 {
		t.Fatalf("got %d, want the default 100", got)
	}
}

func TestParsePageLimit_Explicit(t *testing.T) {
	req := httptest.NewRequest("GET", "/operations?limit=50", nil)
	got, err := parsePageLimit(req, 100, 500)
	if err != nil {
		t.Fatalf("parsePageLimit: %v", err)
	}
	if got != 50 {
		t.Fatalf("got %d, want 50", got)
	}
}

func TestParsePageLimit_Boundaries(t *testing.T) {
	cases := []struct {
		name  string
		limit string
		valid bool
	}{
		{"exactly 1 (minimum)", "1", true},
		{"exactly max", "500", true},
		{"zero", "0", false},
		{"negative", "-1", false},
		{"over max", "501", false},
		{"not a number", "abc", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/operations?limit="+tc.limit, nil)
			_, err := parsePageLimit(req, 100, 500)
			if tc.valid && err != nil {
				t.Errorf("limit=%s: unexpected error %v", tc.limit, err)
			}
			if !tc.valid && err == nil {
				t.Errorf("limit=%s: expected an error, got nil", tc.limit)
			}
		})
	}
}

func TestParseDateRange_Empty(t *testing.T) {
	req := httptest.NewRequest("GET", "/operations", nil)
	from, to, err := parseDateRange(req)
	if err != nil {
		t.Fatalf("parseDateRange: %v", err)
	}
	if from != nil || to != nil {
		t.Fatalf("from=%v to=%v, want both nil when unset", from, to)
	}
}

func TestParseDateRange_BothSet(t *testing.T) {
	req := httptest.NewRequest("GET", "/operations?date_from=2026-01-01&date_to=2026-01-31", nil)
	from, to, err := parseDateRange(req)
	if err != nil {
		t.Fatalf("parseDateRange: %v", err)
	}
	if from == nil || from.String() != "2026-01-01" {
		t.Errorf("from = %v, want 2026-01-01", from)
	}
	if to == nil || to.String() != "2026-01-31" {
		t.Errorf("to = %v, want 2026-01-31", to)
	}
}

func TestParseDateRange_Invalid(t *testing.T) {
	cases := []string{"date_from=not-a-date", "date_to=not-a-date"}
	for _, query := range cases {
		req := httptest.NewRequest("GET", "/operations?"+query, nil)
		if _, _, err := parseDateRange(req); err == nil {
			t.Errorf("query %q: expected an error, got nil", query)
		}
	}
}

func TestCategoryVisibleTo(t *testing.T) {
	viewer := uuid.New()
	accountOwner := uuid.New()
	stranger := uuid.New()

	if categoryVisibleTo(nil, viewer, accountOwner) {
		t.Error("a nil category should never be visible")
	}

	standard := &entities.Category{ID: uuid.New(), UserID: nil, Name: "Groceries"}
	if !categoryVisibleTo(standard, viewer, accountOwner) {
		t.Error("a standard (nil UserID) category should be visible to anyone")
	}

	ownedByViewer := &entities.Category{ID: uuid.New(), UserID: &viewer, Name: "My custom category"}
	if !categoryVisibleTo(ownedByViewer, viewer, accountOwner) {
		t.Error("a category the viewer owns should be visible to them")
	}

	ownedByAccountOwner := &entities.Category{ID: uuid.New(), UserID: &accountOwner, Name: "Owner's custom category"}
	if !categoryVisibleTo(ownedByAccountOwner, viewer, accountOwner) {
		t.Error("a collaborator should be able to use categories the account owner already created")
	}

	ownedByStranger := &entities.Category{ID: uuid.New(), UserID: &stranger, Name: "Someone else's category"}
	if categoryVisibleTo(ownedByStranger, viewer, accountOwner) {
		t.Error("a category owned by an unrelated third user must not be visible")
	}
}

func TestIsSafeRelativeRedirect(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/invite/abc123", true},
		{"/", true},
		{"", false},
		{"//evil.com", false},
		{"http://evil.com", false},
		{"https://evil.com/phishing", false},
		{"relative/no-leading-slash", false},
		{"/../etc/passwd", true}, // still same-origin; not this function's job to resolve path traversal
	}
	for _, tc := range cases {
		if got := isSafeRelativeRedirect(tc.path); got != tc.want {
			t.Errorf("isSafeRelativeRedirect(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
