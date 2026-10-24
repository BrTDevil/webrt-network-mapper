// Package web conține frontend-ul inclus în binar.
package web

import "embed"

//go:embed index.html icons.json css js
var FS embed.FS
