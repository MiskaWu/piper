package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/miskawu/piper/internal/guard"
)

// resolvePiperHost extracts the first non-loopback bind address from the --host flag
// and combines it with the port to produce an address DevContainers can use to reach Piper.
// Returns "" if all addresses are loopback (containers cannot reach Piper via API).
func resolvePiperHost(hostFlag string, port int) string {
	for _, h := range strings.Split(hostFlag, ",") {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		ip := net.ParseIP(h)
		if ip == nil || ip.IsLoopback() {
			continue
		}
		return fmt.Sprintf("%s:%d", h, port)
	}
	return ""
}

// buildServers creates one HTTP server per bind address. Every server sits
// behind the guard; a non-loopback address also requires the startup token.
func buildServers(hosts []string, port int, handler http.Handler, g *guard.Guard) []*http.Server {
	var servers []*http.Server
	for _, h := range hosts {
		servers = append(servers, &http.Server{
			Addr:    net.JoinHostPort(h, fmt.Sprint(port)),
			Handler: g.Wrap(handler, !guard.IsLoopback(h)),
		})
	}
	return servers
}

// printStartupURL shows the token exactly once. It writes straight to the
// terminal rather than through log, so the token stays out of log files.
func printStartupURL(w io.Writer, token, piperHost string) {
	if token == "" {
		return
	}
	fmt.Fprintf(w, "\npiper is reachable from the network, so a token is required.\n"+
		"Open: http://%s/?%s=%s\n"+
		"The token is shown only here and changes on every start.\n\n", piperHost, guard.TokenParam, token)
}

// parseHosts splits a comma-separated host flag into individual bind addresses.
// Each entry must be a valid IPv4/IPv6 address. Returns an error if any entry is invalid.
func parseHosts(flag string) ([]string, error) {
	var hosts []string
	seen := make(map[string]bool)
	for _, h := range strings.Split(flag, ",") {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if net.ParseIP(h) == nil {
			return nil, fmt.Errorf("invalid bind address: %q", h)
		}
		if seen[h] {
			continue
		}
		seen[h] = true
		hosts = append(hosts, h)
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no valid bind address provided")
	}
	return hosts, nil
}
