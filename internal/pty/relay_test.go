package pty

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPTYRelay_Echo(t *testing.T) {
	// Use "cat" as a simple echo command (it echoes stdin to stdout)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cmd := exec.Command("cat")
		HandleTerminalWSWithCmd(w, r, cmd)
	}))
	defer srv.Close()

	// Connect via WebSocket
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Write some text to the PTY via WebSocket
	input := "hello pty\n"
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte(input)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read back from WebSocket — cat should echo the input
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var accumulated string
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		accumulated += string(data)
		if strings.Contains(accumulated, "hello pty") {
			break
		}
	}

	if !strings.Contains(accumulated, "hello pty") {
		t.Errorf("expected echo to contain %q, got %q", "hello pty", accumulated)
	}
}

func TestPTYRelay_Resize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cmd := exec.Command("cat")
		HandleTerminalWSWithCmd(w, r, cmd)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send a resize message — should not crash or be written to PTY
	resizeJSON := `{"cols":120,"rows":40}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(resizeJSON)); err != nil {
		t.Fatalf("write resize: %v", err)
	}

	// Send actual data after resize
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("after resize\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var accumulated string
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		accumulated += string(data)
		if strings.Contains(accumulated, "after resize") {
			break
		}
	}

	// The resize JSON should NOT appear in the output — only "after resize" should
	if strings.Contains(accumulated, `"cols"`) {
		t.Errorf("resize message was written to PTY instead of being handled as resize")
	}
}
