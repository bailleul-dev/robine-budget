package web

import "embed"

// distFS embeds the built web app into the server binary — see static.go
// and the Dockerfile's web-builder stage, which exports mobile/ (Expo) as a
// web SPA and overwrites this directory with the real assets before
// compiling. In local dev this directory holds only a placeholder (the web
// app runs separately via `expo start --web` or `npm run dev` in frontend/).
//
//go:embed all:dist
var distFS embed.FS
