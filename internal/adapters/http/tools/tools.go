package tools

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed pages/*.html
var pagesFS embed.FS

func Handler() http.Handler {
	pages, err := fs.Sub(pagesFS, "pages")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/tools/", http.FileServer(http.FS(pages)))
}
