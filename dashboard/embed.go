package dashboard

import (
	"embed"
	"io/fs"

	"github.com/ayushkunwarsingh/forge/server"
	"github.com/ayushkunwarsingh/forge/storage"
)

//go:embed *.html *.css *.js
var dashboardFS embed.FS

// RegisterRoutes sets up the endpoint to serve the embedded dashboard.
func RegisterRoutes(router *server.Router) error {
	// We need to serve the embedded files as a sub FS
	staticFS, err := fs.Sub(dashboardFS, ".")
	if err != nil {
		return err
	}

	// Catch-all handler for the dashboard UI
	router.GET("/dashboard/*path", func(ctx *server.Context) {
		p := ctx.Param("path")
		if p == "" || p == "/" {
			p = "index.html"
		}

		// Try to serve the file
		data, err := fs.ReadFile(staticFS, p)
		if err != nil {
			// SPA Fallback
			data, err = fs.ReadFile(staticFS, "index.html")
			if err != nil {
				ctx.Error(404, "Dashboard asset not found")
				return
			}
			p = "index.html"
		}

		contentType := storage.DetectMIME(p, data)
		ctx.Status(200)
		ctx.SetResponseHeader("Content-Type", contentType)
		ctx.Response.SetBody(data)
	})

	// The router strips trailing slashes in splitPath, so BOTH /dashboard and /dashboard/ 
	// match this route. We will serve index.html directly from this route.
	router.GET("/dashboard", func(ctx *server.Context) {
		data, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			ctx.Error(404, "Dashboard asset not found")
			return
		}
		ctx.Status(200)
		ctx.SetResponseHeader("Content-Type", "text/html")
		ctx.Response.SetBody(data)
	})

	return nil
}
