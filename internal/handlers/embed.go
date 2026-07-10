package handlers

import "embed"

//go:embed web/dist/*
var frontendAssets embed.FS
