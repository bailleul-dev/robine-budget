package core

// Set via -ldflags at build time (see the Dockerfile's backend-builder
// stage and the Makefile's docker-build/build-and-push-docker targets,
// which derive them from git). Local `go run`/`make backend-build` without
// those flags keeps these defaults, which is itself a useful signal that
// you're looking at an ad-hoc dev build rather than a shipped one.
var (
	CommitHash = "dev"
	Branch     = "dev"
	BuildDate  = "unknown"
)
