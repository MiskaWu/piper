package guard

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const port = 8622

func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
}

func request(host, target string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Host = host
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestLoopbackOnlyHasNoToken(t *testing.T) {
	g, err := New([]string{"127.0.0.1"}, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g.Token() != "" {
		t.Error("loopback-only bind must not generate a token")
	}
	// A candidate must never match the empty token.
	if g.matches("") {
		t.Error("empty token must not match")
	}
}

func TestNonLoopbackGeneratesFreshToken(t *testing.T) {
	a, err := New([]string{"127.0.0.1", "192.0.2.10"}, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := New([]string{"192.0.2.10"}, port, nil)
	if len(a.Token()) != 64 || a.Token() == b.Token() {
		t.Errorf("tokens = %q, %q; want two different 64-char values", a.Token(), b.Token())
	}
}

func TestUnspecifiedBindNeedsToken(t *testing.T) {
	g, err := New([]string{"0.0.0.0"}, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g.Token() == "" {
		t.Error("0.0.0.0 is reachable from the network and must generate a token")
	}
}

func TestInvalidInput(t *testing.T) {
	if _, err := New([]string{"not-an-ip"}, port, nil); err == nil {
		t.Error("invalid bind address must fail")
	}
	if _, err := New([]string{"127.0.0.1"}, port, []string{"localhost:5173"}); err == nil {
		t.Error("origin without a scheme must fail")
	}
}

func TestHostAndOrigin(t *testing.T) {
	g, err := New([]string{"127.0.0.1"}, port, []string{"http://localhost:5173"})
	if err != nil {
		t.Fatal(err)
	}
	h := g.Wrap(ok(), false)
	ws := map[string]string{"Upgrade": "websocket"}
	merge := func(origin string) map[string]string {
		return map[string]string{"Upgrade": "websocket", "Origin": origin}
	}
	cases := []struct {
		name    string
		host    string
		headers map[string]string
		want    int
	}{
		{"curl on loopback", "127.0.0.1:8622", nil, http.StatusTeapot},
		{"localhost name", "localhost:8622", nil, http.StatusTeapot},
		{"own UI over WebSocket", "127.0.0.1:8622", merge("http://127.0.0.1:8622"), http.StatusTeapot},
		{"own UI via localhost", "localhost:8622", merge("http://localhost:8622"), http.StatusTeapot},
		{"Vite dev server", "localhost:5173", merge("http://localhost:5173"), http.StatusTeapot},
		{"cross-origin WebSocket", "127.0.0.1:8622", merge("https://evil.example"), http.StatusForbidden},
		{"same host, other port", "127.0.0.1:8622", merge("http://127.0.0.1:9999"), http.StatusForbidden},
		{"sandboxed iframe", "127.0.0.1:8622", merge("null"), http.StatusForbidden},
		{"WebSocket without Origin", "127.0.0.1:8622", ws, http.StatusForbidden},
		{"cross-origin POST to the API", "127.0.0.1:8622", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"DNS rebinding", "evil.example:8622", nil, http.StatusForbidden},
		{"DNS rebinding keeps Origin in step with Host", "evil.example:8622", merge("http://evil.example:8622"), http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := serve(h, request(c.host, "/ws/host-shell", c.headers)).Code; got != c.want {
				t.Errorf("status = %d, want %d", got, c.want)
			}
		})
	}
}

func TestNonLoopbackRequiresToken(t *testing.T) {
	g, err := New([]string{"127.0.0.1", "192.0.2.10"}, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := g.Wrap(ok(), true)
	const host = "192.0.2.10:8622"
	own := "http://" + host
	cases := []struct {
		name    string
		target  string
		headers map[string]string
		want    int
	}{
		{"no token", "/", nil, http.StatusUnauthorized},
		{"API without token", "/api/sessions", nil, http.StatusUnauthorized},
		{"wrong token", "/?token=nope", nil, http.StatusUnauthorized},
		{"wrong cookie", "/", map[string]string{"Cookie": CookieName + "=nope"}, http.StatusUnauthorized},
		{"WebSocket without token", "/ws/host-shell", map[string]string{"Upgrade": "websocket", "Origin": own}, http.StatusUnauthorized},
		{"bearer", "/api/sessions", map[string]string{"Authorization": "Bearer " + g.Token()}, http.StatusTeapot},
		{"cookie", "/", map[string]string{"Cookie": CookieName + "=" + g.Token()}, http.StatusTeapot},
		{"WebSocket with cookie", "/ws/host-shell", map[string]string{"Upgrade": "websocket", "Origin": own, "Cookie": CookieName + "=" + g.Token()}, http.StatusTeapot},
		{"WebSocket with query token", "/ws/host-shell?token=" + g.Token(), map[string]string{"Upgrade": "websocket", "Origin": own}, http.StatusTeapot},
		// A token does not excuse a page from another origin.
		{"cross-origin with token", "/ws/host-shell?token=" + g.Token(), map[string]string{"Upgrade": "websocket", "Origin": "https://evil.example"}, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := serve(h, request(host, c.target, c.headers)).Code; got != c.want {
				t.Errorf("status = %d, want %d", got, c.want)
			}
		})
	}
}

func TestStartupURLBecomesCookie(t *testing.T) {
	g, err := New([]string{"192.0.2.10"}, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := serve(g.Wrap(ok(), true), request("192.0.2.10:8622", "/epics?tab=a&token="+g.Token(), nil))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want redirect", w.Code)
	}
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil || loc.Path != "/epics" || loc.Query().Get("tab") != "a" || loc.Query().Has(TokenParam) {
		t.Errorf("Location = %q, want the same page without the token", w.Header().Get("Location"))
	}
	cookie := w.Header().Get("Set-Cookie")
	for _, want := range []string{CookieName + "=" + g.Token(), "HttpOnly", "SameSite=Strict"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("Set-Cookie = %q, want it to contain %q", cookie, want)
		}
	}
}

func TestLoopbackListenerIgnoresToken(t *testing.T) {
	g, err := New([]string{"127.0.0.1", "192.0.2.10"}, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := serve(g.Wrap(ok(), false), request("127.0.0.1:8622", "/", nil)).Code; got != http.StatusTeapot {
		t.Errorf("status = %d, want the loopback listener to work without a token", got)
	}
}

func TestIsLoopback(t *testing.T) {
	for host, want := range map[string]bool{"127.0.0.1": true, "::1": true, "192.0.2.10": false, "0.0.0.0": false, "": false} {
		if got := IsLoopback(host); got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}
