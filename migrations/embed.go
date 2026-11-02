// Package migrations embeds the immutable, numbered SQL migration pairs.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
