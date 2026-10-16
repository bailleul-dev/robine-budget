package web

import (
	"net/http"

	"bailleul.dev/budget-server/internal/core"
	"bailleul.dev/budget-server/pkg/httpx"
)

type versionResponse struct {
	Commit    string `json:"commit"`
	Branch    string `json:"branch"`
	BuildDate string `json:"build_date"`
}

// VersionHandler exposes the build stamp set via -ldflags (see
// internal/core.CommitHash/Branch/BuildDate) — backs the frontend's
// /version page. Public: it's diagnostic info, not sensitive.
func VersionHandler(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, versionResponse{
		Commit:    core.CommitHash,
		Branch:    core.Branch,
		BuildDate: core.BuildDate,
	})
}
