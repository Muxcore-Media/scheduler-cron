package server

import (
	"net"
	"net/http"
	"strings"
)

// IsLoopbackBind reports whether addr listens on loopback only.
func IsLoopbackBind(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return false
		}
		host = addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return strings.EqualFold(host, "localhost")
	}
	return ip.IsLoopback()
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.apiToken == "" {
		return true
	}
	got := strings.TrimSpace(r.Header.Get("X-Scheduler-Token"))
	if got == "" {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
			got = strings.TrimSpace(h[7:])
		}
	}
	if got != s.apiToken {
		writeError(w, http.StatusUnauthorized, "missing or invalid scheduler API token")
		return false
	}
	return true
}
