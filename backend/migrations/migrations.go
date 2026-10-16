// Package migrations embeds the SQL migration files into the server binary
// so they can be applied at startup regardless of the process's working
// directory (dev via air, a built binary, or a container).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
