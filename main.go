package main

import (
	"embed"

	"llmrelay/backend"
)

//go:embed frontend/pages/* frontend/assets/*
var frontendFS embed.FS

func main() {
	backend.Run(frontendFS)
}
