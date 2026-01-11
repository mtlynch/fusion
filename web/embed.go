package web

import (
	"embed"
	"io/fs"
)

//go:embed templates static
var assets embed.FS

var Templates fs.FS
var Static fs.FS

func init() {
	templates, err := fs.Sub(assets, "templates")
	if err != nil {
		panic(err)
	}
	Templates = templates

	static, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	Static = static
}
