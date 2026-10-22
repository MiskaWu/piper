package ws

import (
	"net/http/httptest"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dialWS(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(url, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func TestHub_BroadcastToMultipleClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	srv := httptest.NewServer(hub)
	defer srv.Close()

	c1 := dialWS(t, srv.URL)
	defer c1.Close()
	c2 := dialWS(t, srv.URL)
	defer c2.Close()

	// Give the hub a moment to register both clients.
	time.Sleep(50 * time.Millisecond)

	msg := []byte(`{"event":"update"}`)
	hub.Broadcast(msg)

	for _, c := range []*websocket.Conn{c1, c2} {
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, got, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(got) != string(msg) {
			t.Errorf("got %q, want %q", got, msg)
		}
	}
}

func TestHub_ClientDisconnect(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	srv := httptest.NewServer(hub)
	defer srv.Close()

	c1 := dialWS(t, srv.URL)
	// Close immediately to simulate disconnect.
	c1.Close()

	// Give the hub a moment to process the unregister.
	time.Sleep(50 * time.Millisecond)

	// Broadcast should not panic or error.
	hub.Broadcast([]byte(`{"event":"ping"}`))

	// Allow broadcast to be processed.
	time.Sleep(50 * time.Millisecond)

	// Verify hub still works by connecting a new client and broadcasting.
	c2 := dialWS(t, srv.URL)
	defer c2.Close()

	time.Sleep(50 * time.Millisecond)

	msg := []byte(`{"event":"after_disconnect"}`)
	hub.Broadcast(msg)

	c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, got, err := c2.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(msg) {
		t.Errorf("got %q, want %q", got, msg)
	}
}

func TestHub_SlowClientDoesNotBlockBroadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	srv := httptest.NewServer(hub)
	defer srv.Close()

	// c1: slow client — connect but never read
	c1 := dialWS(t, srv.URL)
	defer c1.Close()

	// c2: healthy client
	c2 := dialWS(t, srv.URL)
	defer c2.Close()

	time.Sleep(50 * time.Millisecond)

	// Flood broadcasts — if write deadline works, slow client gets kicked
	// instead of blocking the Run loop forever.
	for i := 0; i < 10; i++ {
		hub.Broadcast([]byte(fmt.Sprintf(`{"i":%d}`, i)))
	}

	// Healthy client should still receive messages within a reasonable time.
	c2.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, got, err := c2.ReadMessage()
	if err != nil {
		t.Fatalf("healthy client should receive message but got error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("healthy client received empty message")
	}
}

func TestHub_BroadcastNonBlocking(t *testing.T) {
	hub := NewHub()
	// Do NOT start Run — simulates a stuck Run loop.

	// Broadcast should not block even if Run loop is not consuming.
	done := make(chan struct{})
	go func() {
		// Fill beyond the channel buffer (256) + extra
		for i := 0; i < 300; i++ {
			hub.Broadcast([]byte(`{"i":1}`))
		}
		close(done)
	}()

	select {
	case <-done:
		// OK — Broadcast did not block
	case <-time.After(3 * time.Second):
		t.Fatal("Broadcast blocked when channel is full — should be non-blocking")
	}
}

func TestHub_RegisterNonBlocking(t *testing.T) {
	hub := NewHub()
	// Do NOT start Run — register channel should be buffered.

	srv := httptest.NewServer(hub)
	defer srv.Close()

	// Multiple clients should be able to upgrade without Run consuming register channel.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 8; i++ {
			c := dialWS(t, srv.URL)
			c.Close()
		}
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("HandleWebSocket blocked on register — channel should be buffered")
	}
}

func TestHub_WriteDeadlineEvictsDeadClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	srv := httptest.NewServer(hub)
	defer srv.Close()

	// c1: connect but never read — will fill its TCP buffer and become "slow"
	c1 := dialWS(t, srv.URL)
	defer c1.Close()

	// c2: healthy client that reads promptly
	c2 := dialWS(t, srv.URL)
	defer c2.Close()

	time.Sleep(50 * time.Millisecond)

	// Send many large messages to saturate c1's TCP buffer
	bigMsg := []byte(strings.Repeat("x", 32*1024))
	for i := 0; i < 50; i++ {
		hub.Broadcast(bigMsg)
	}

	// After write deadline kicks in, c1 should be evicted.
	// c2 should still work.
	time.Sleep(1 * time.Second)

	testMsg := []byte(`{"alive":true}`)
	hub.Broadcast(testMsg)

	c2.SetReadDeadline(time.Now().Add(10 * time.Second))
	// Drain any queued messages until we find our test message
	for {
		_, got, err := c2.ReadMessage()
		if err != nil {
			t.Fatalf("healthy client read error: %v", err)
		}
		if string(got) == string(testMsg) {
			break
		}
	}
}
