// Package guard is the one gate every request passes before it reaches a route.
//
// Piper can open a shell, attach to any tmux session and move files, so a
// request is only let through when it provably comes from Piper's own UI or
// from a tool that was handed the token. Three checks, in order:
//
//  1. Host must be an address Piper answers to. This stops DNS rebinding, where
//     a page on evil.example re-points its own name at 127.0.0.1 and then counts
//     as same-origin with Piper.
//  2. Origin, when present, must be one of those same addresses. Browsers always
//     send it on WebSocket handshakes and cross-site POSTs, so this is what stops
//     an arbitrary web page from reaching ws://127.0.0.1:8622/ws/host-shell. A
//     WebSocket handshake with no Origin at all is refused too.
//  3. On a non-loopback listener the startup token is required as well, since
//     anyone on the network can reach it.
//
// The allowed addresses come from the bind flags, not from the request: echoing
// r.Host back would make the Origin check pass for a rebound name.
package guard

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	// CookieName holds the token once a browser has presented it. It is
	// HttpOnly, and WebSocket handshakes carry it without any UI change.
	CookieName = "piper_token"
	// TokenParam is the query parameter of the URL printed at startup.
	TokenParam = "token"
)

// Guard decides which requests may reach Piper's routes.
type Guard struct {
	authorities map[string]bool // lower-cased host:port Piper answers to
	token       string          // empty when every listener is loopback
}

// New builds a Guard for the given bind addresses. A token is generated only
// when at least one of them is reachable from outside this machine.
// extraOrigins lists further origins to trust, such as the Vite dev server.
func New(hosts []string, port int, extraOrigins []string) (*Guard, error) {
	g := &Guard{authorities: map[string]bool{}}
	needToken := false
	for _, h := range hosts {
		ip := net.ParseIP(h)
		if ip == nil {
			return nil, fmt.Errorf("invalid bind address: %q", h)
		}
		switch {
		case ip.IsLoopback():
			g.addLoopback(port)
		case ip.IsUnspecified():
			needToken = true
			g.addLoopback(port)
			addrs, err := net.InterfaceAddrs()
			if err != nil {
				return nil, fmt.Errorf("list interface addresses: %w", err)
			}
			for _, a := range addrs {
				if n, ok := a.(*net.IPNet); ok {
					g.addAuthority(n.IP.String(), port)
				}
			}
		default:
			needToken = true
			g.addAuthority(h, port)
		}
	}
	for _, o := range extraOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		u, err := url.Parse(o)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("invalid origin: %q (want e.g. http://localhost:5173)", o)
		}
		g.authorities[strings.ToLower(u.Host)] = true
	}
	if needToken {
		token, err := newToken()
		if err != nil {
			return nil, err
		}
		g.token = token
	}
	return g, nil
}

func (g *Guard) addLoopback(port int) {
	for _, h := range []string{"127.0.0.1", "localhost", "::1"} {
		g.addAuthority(h, port)
	}
}

func (g *Guard) addAuthority(host string, port int) {
	g.authorities[strings.ToLower(net.JoinHostPort(host, strconv.Itoa(port)))] = true
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Token returns the startup token, or "" when no listener needs one.
func (g *Guard) Token() string { return g.token }

// IsLoopback reports whether a bind address is only reachable from this machine.
func IsLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Wrap puts the checks in front of next. requireToken is set for listeners
// bound to a non-loopback address.
func (g *Guard) Wrap(next http.Handler, requireToken bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.authorities[strings.ToLower(r.Host)] {
			http.Error(w, "forbidden: unknown host", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		upgrade := isWebSocket(r)
		if (origin == "" && upgrade) || (origin != "" && !g.originAllowed(origin)) {
			http.Error(w, "forbidden: cross-origin request", http.StatusForbidden)
			return
		}
		if requireToken {
			if !upgrade && g.acceptQueryToken(w, r) {
				return
			}
			if !g.hasToken(r) {
				http.Error(w, "unauthorized: open the URL printed at startup", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Guard) originAllowed(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return g.authorities[strings.ToLower(u.Host)]
}

// acceptQueryToken turns the token in the startup URL into a cookie, then
// redirects so the token does not stay in the address bar or browser history.
// It reports whether it has written a response.
func (g *Guard) acceptQueryToken(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()
	if !g.matches(q.Get(TokenParam)) || r.Method != http.MethodGet {
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    g.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	q.Del(TokenParam)
	target := url.URL{Path: r.URL.Path, RawQuery: q.Encode()}
	http.Redirect(w, r, target.String(), http.StatusFound)
	return true
}

func (g *Guard) hasToken(r *http.Request) bool {
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && g.matches(bearer) {
		return true
	}
	if c, err := r.Cookie(CookieName); err == nil && g.matches(c.Value) {
		return true
	}
	return g.matches(r.URL.Query().Get(TokenParam))
}

func (g *Guard) matches(candidate string) bool {
	return g.token != "" && subtle.ConstantTimeCompare([]byte(candidate), []byte(g.token)) == 1
}

func isWebSocket(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
