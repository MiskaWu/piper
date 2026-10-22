package store

import (
	"regexp"
	"strings"
)

var nonAlphanumDash = regexp.MustCompile(`[^a-z0-9-]+`)
var multiDash = regexp.MustCompile(`-{2,}`)

// PathToSlug generates a deterministic slug from an absolute filesystem path.
// Example: "/home/user/projects/acme/server" → "home-user-projects-acme-server"
func PathToSlug(path string) string {
	s := strings.TrimPrefix(path, "/")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ToLower(s)
	s = nonAlphanumDash.ReplaceAllString(s, "-")
	s = multiDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
