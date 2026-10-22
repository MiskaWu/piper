package pty

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// The relay must not depend on the guard being in front of it: on its own it
// still refuses a handshake whose Origin differs from Host.
func TestRelayRefusesCrossOriginOnItsOwn(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleTerminalWSWithCmd(w, r, exec.Command("cat"))
	}))
	defer ts.Close()

	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {"https://evil.example"}})
	if conn != nil {
		conn.Close()
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("resp = %+v, err = %v; want 403", resp, err)
	}
}
