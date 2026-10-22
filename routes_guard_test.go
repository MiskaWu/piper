package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/miskawu/piper/internal/guard"
)

// guardedServer runs the real routes behind the guard on a known port.
func guardedServer(t *testing.T, hosts []string, requireToken bool) (*httptest.Server, *guard.Guard, int) {
	t.Helper()
	routes := setupTestServer(t, &mockCommander{}).Config.Handler
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	g, err := guard.New(hosts, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(g.Wrap(routes, requireToken))
	ts.Listener.Close()
	ts.Listener = ln
	ts.Start()
	t.Cleanup(ts.Close)
	return ts, g, port
}

// dial attempts a WebSocket handshake and returns the HTTP status it got.
func dial(t *testing.T, ts *httptest.Server, path string, header http.Header) int {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + path
	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if conn != nil {
		defer conn.Close()
	}
	if resp == nil {
		t.Fatalf("dial %s: %v", path, err)
	}
	return resp.StatusCode
}

func TestCrossOriginWebSocketIsRejected(t *testing.T) {
	ts, _, port := guardedServer(t, []string{"127.0.0.1"}, false)
	for _, path := range []string{"/ws/host-shell", "/ws/terminal/demo", "/ws/status"} {
		t.Run(path, func(t *testing.T) {
			for _, origin := range []string{"https://evil.example", fmt.Sprintf("http://127.0.0.1:%d", port+1), "null"} {
				if got := dial(t, ts, path, http.Header{"Origin": {origin}}); got != http.StatusForbidden {
					t.Errorf("Origin %s: status = %d, want 403", origin, got)
				}
			}
			if got := dial(t, ts, path, nil); got != http.StatusForbidden {
				t.Errorf("no Origin: status = %d, want 403", got)
			}
		})
	}
}

func TestSameOriginWebSocketIsAccepted(t *testing.T) {
	ts, _, port := guardedServer(t, []string{"127.0.0.1"}, false)
	origin := http.Header{"Origin": {fmt.Sprintf("http://127.0.0.1:%d", port)}}
	if got := dial(t, ts, "/ws/status", origin); got != http.StatusSwitchingProtocols {
		t.Errorf("status = %d, want 101", got)
	}
}

func TestCrossOriginAPIRequestIsRejected(t *testing.T) {
	ts, _, _ := guardedServer(t, []string{"127.0.0.1"}, false)
	// text/plain needs no CORS preflight, so a browser would send this from any page.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/files/move", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

func TestNonLoopbackWebSocketRequiresToken(t *testing.T) {
	ts, g, port := guardedServer(t, []string{"127.0.0.1", "192.0.2.10"}, true)
	origin := fmt.Sprintf("http://127.0.0.1:%d", port)
	for _, path := range []string{"/ws/host-shell", "/ws/terminal/demo", "/ws/status"} {
		if got := dial(t, ts, path, http.Header{"Origin": {origin}}); got != http.StatusUnauthorized {
			t.Errorf("%s without token: status = %d, want 401", path, got)
		}
		wrong := http.Header{"Origin": {origin}, "Cookie": {guard.CookieName + "=nope"}}
		if got := dial(t, ts, path, wrong); got != http.StatusUnauthorized {
			t.Errorf("%s with a wrong token: status = %d, want 401", path, got)
		}
	}
	withCookie := http.Header{"Origin": {origin}, "Cookie": {guard.CookieName + "=" + g.Token()}}
	if got := dial(t, ts, "/ws/status", withCookie); got != http.StatusSwitchingProtocols {
		t.Errorf("with token: status = %d, want 101", got)
	}
}

func TestBuildServersGuardsEachBindAddress(t *testing.T) {
	const port = 8622
	routes := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	g, err := guard.New([]string{"127.0.0.1", "192.0.2.10"}, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	servers := buildServers([]string{"127.0.0.1", "192.0.2.10"}, port, routes, g)
	if len(servers) != 2 || servers[0].Addr != "127.0.0.1:8622" || servers[1].Addr != "192.0.2.10:8622" {
		t.Fatalf("servers = %+v", servers)
	}

	serve := func(srv *http.Server, host, token string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
		r.Host = host
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, r)
		return w.Code
	}
	if got := serve(servers[0], "127.0.0.1:8622", ""); got != http.StatusTeapot {
		t.Errorf("loopback without token: status = %d", got)
	}
	if got := serve(servers[1], "192.0.2.10:8622", ""); got != http.StatusUnauthorized {
		t.Errorf("non-loopback without token: status = %d, want 401", got)
	}
	if got := serve(servers[1], "192.0.2.10:8622", g.Token()); got != http.StatusTeapot {
		t.Errorf("non-loopback with token: status = %d", got)
	}
}

func TestPrintStartupURL(t *testing.T) {
	var buf bytes.Buffer
	printStartupURL(&buf, "", "192.0.2.10:8622")
	if buf.Len() != 0 {
		t.Errorf("no token must print nothing, got %q", buf.String())
	}
	printStartupURL(&buf, "abc", "192.0.2.10:8622")
	if !strings.Contains(buf.String(), "http://192.0.2.10:8622/?token=abc") {
		t.Errorf("output = %q", buf.String())
	}
}
