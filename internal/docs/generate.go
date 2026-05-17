package docs

import "embed"

//go:embed cli_reference.md
var referenceFS embed.FS

func Generate() string {
	data, err := referenceFS.ReadFile("cli_reference.md")
	if err != nil {
		return ""
	}
	return string(data)
}