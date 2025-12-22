package web_frontend

import (
	"embed"
)

//go:embed web/dist/**
var EmbeddedFS embed.FS
