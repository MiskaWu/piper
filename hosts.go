package main

import (
	"fmt"
	"net"
	"strings"
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
