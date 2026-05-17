package docs

import _ "embed"

//go:embed cli_reference.md
var reference string

func EmbeddedReference() string {
	return reference
}