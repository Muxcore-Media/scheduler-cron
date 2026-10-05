package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// IsLoopbackAddr reports whether a listen address binds only to a loopback
// interface. Empty host (":9200"), wildcard hosts and hostnames other than
// "localhost" are treated as non-loopback.
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ValidateListen enforces NFR-SEC-011: a non-loopback listen address requires
// a bearer token.
func ValidateListen(addr, token string) error {
	if !IsLoopbackAddr(addr) && token == "" {
		return fmt.Errorf("refusing to listen on non-loopback address %q without SCHEDULER_HTTP_TOKEN (set a bearer token or bind to 127.0.0.1)", addr)
	}
	return nil
}

// TokenMatches compares a presented token with the configured one in constant time.
func TokenMatches(want, got string) bool {
	a := sha256.Sum256([]byte(want))
	b := sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func bearer(h string) string {
	const p = "bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

// RequireToken wraps next so every path except GET/HEAD /health needs
// "Authorization: Bearer <token>". An empty token disables the check.
func RequireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		got := bearer(r.Header.Get("Authorization"))
		if got == "" || !TokenMatches(token, got) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="scheduler-cron"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GRPCAuth returns server options requiring the bearer token in the
// "authorization" metadata. Empty token yields no options.
func GRPCAuth(token string) []grpc.ServerOption {
	if token == "" {
		return nil
	}
	check := func(ctx context.Context) error {
		md, _ := metadata.FromIncomingContext(ctx)
		for _, v := range md.Get("authorization") {
			if g := bearer(v); g != "" && TokenMatches(token, g) {
				return nil
			}
		}
		return status.Error(codes.Unauthenticated, "unauthorized")
	}
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
			if err := check(ctx); err != nil {
				return nil, err
			}
			return h(ctx, req)
		}),
		grpc.ChainStreamInterceptor(func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
			if err := check(ss.Context()); err != nil {
				return err
			}
			return h(srv, ss)
		}),
	}
}
