package server

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

func webhookAllowPrivate() bool {
	return os.Getenv("SCHEDULER_WEBHOOK_ALLOW_PRIVATE") == "1"
}

func validateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("webhook URL scheme %q not allowed (http/https only)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("webhook URL missing host")
	}
	if webhookAllowPrivate() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return blockPrivateHost(ctx, u.Hostname())
}

func blockPrivateHost(ctx context.Context, host string) error {
	hostOnly := stripPort(host)
	if ip := net.ParseIP(hostOnly); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("webhook host %q resolves to private IP (set SCHEDULER_WEBHOOK_ALLOW_PRIVATE=1 to override)", hostOnly)
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, hostOnly)
	if err != nil {
		return fmt.Errorf("webhook host lookup failed for %q: %w", hostOnly, err)
	}
	for _, ia := range addrs {
		if isPrivateIP(ia.IP) {
			return fmt.Errorf("webhook host %q resolves to private IP %q (set SCHEDULER_WEBHOOK_ALLOW_PRIVATE=1 to override)", hostOnly, ia.IP)
		}
	}
	return nil
}

func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}
