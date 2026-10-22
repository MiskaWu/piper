package devcontainer

import (
	"embed"
	"io/fs"
)

//go:embed templates/*
var templateFS embed.FS

// TemplateFS returns the embedded template filesystem.
func TemplateFS() fs.FS { return templateFS }
