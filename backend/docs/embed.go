package docs

import "embed"

// FS embeds the OpenAPI 3.1 contract served at /docs/openapi.yaml.
//
//go:embed openapi.yaml
var FS embed.FS
