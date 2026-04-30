package docs

import _ "embed"

//go:embed cli_reference.md
var embeddedReference string

// EmbeddedReference returns the embedded generated CLI reference documentation.
func EmbeddedReference() string {
	return embeddedReference
}
