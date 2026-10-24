// Package assets conține fișierele de date incluse în binar.
package assets

import _ "embed"

// OUI este registrul IEEE MA-L compactat: prefix (6 hex), vendor.
//
//go:embed oui.csv
var OUI []byte
