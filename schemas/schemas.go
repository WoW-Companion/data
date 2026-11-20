// Package schemas embeds the JSON Schemas for the published files.
package schemas

import _ "embed"

// Manifest is the JSON Schema for a dataset manifest.
//
//go:embed manifest.schema.json
var Manifest []byte
