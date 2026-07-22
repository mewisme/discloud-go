package server

import (
	"net/http"
	"strings"

	"github.com/mewisme/discloud-go/internal/server/install"
)

func (s *Server) handleInstallSH(w http.ResponseWriter, r *http.Request) {
	s.writeInstall(w, r, install.ScriptSH, "text/x-shellscript; charset=utf-8")
}

func (s *Server) handleInstallPS1(w http.ResponseWriter, r *http.Request) {
	s.writeInstall(w, r, install.ScriptPS1, "text/plain; charset=utf-8")
}

func (s *Server) writeInstall(w http.ResponseWriter, r *http.Request, tmpl, contentType string) {
	base := s.baseURL(r)
	origin := strings.TrimRight(s.webOrigin, "/")
	body := strings.ReplaceAll(tmpl, "{{DISCLOUD_BASE}}", base)
	body = strings.ReplaceAll(body, "{{DISCLOUD_ORIGIN}}", origin)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}
