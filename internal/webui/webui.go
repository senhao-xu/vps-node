package webui

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
)

const fallbackHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>vps-node panel</title></head>
<body style="font-family: sans-serif; max-width: 40rem; margin: 4rem auto;">
<h1>Web UI not embedded</h1>
<p>This panel binary was built without the web UI. Run:</p>
<pre><code>cd web &amp;&amp; npm ci &amp;&amp; npm run build
go build -tags embed_ui -o panel ./cmd/panel</code></pre>
<p>The Admin API is served under <code>/api</code> regardless.</p>
</body></html>
`

func Embedded() bool {
	return embedded
}

func Wrap(api http.Handler, logger *slog.Logger) http.Handler {
	sub, ok := uiFS()
	if !ok {
		if logger != nil {
			logger.Info("web UI not embedded; serving API only", "hint", "npm run build + go build -tags embed_ui")
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" || r.URL.Path == "/index.html" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, fallbackHTML)
				return
			}
			api.ServeHTTP(w, r)
		})
	}

	var index []byte
	if data, err := fs.ReadFile(sub, "index.html"); err == nil {
		index = data
	} else {
		index = []byte(fallbackHTML)
	}
	fileServer := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/api" || strings.HasPrefix(p, "/api/") || p == "/healthz" {
			api.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(p, "/")
		if name != "" && name != "index.html" {
			if f, err := sub.Open(name); err == nil {
				_ = f.Close()
				if strings.HasPrefix(p, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
