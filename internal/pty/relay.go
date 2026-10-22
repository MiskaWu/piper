package pty

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os/exec"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type resizeMsg struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleTerminalWS upgrades an HTTP request to a WebSocket and relays I/O
// between the WebSocket and a tmux attach-session command running in a PTY.
func HandleTerminalWS(w http.ResponseWriter, r *http.Request, sessionName string) {
	cmd := exec.Command("tmux", "attach-session", "-t", sessionName)
	HandleTerminalWSWithCmd(w, r, cmd)
}

// HandleTerminalWSWithCmd upgrades an HTTP request to a WebSocket and relays
// I/O between the WebSocket and the given command running in a PTY.
func HandleTerminalWSWithCmd(w http.ResponseWriter, r *http.Request, cmd *exec.Cmd) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		log.Printf("pty start: %v", err)
		conn.Close()
		return
	}

	defer func() {
		ptmx.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		conn.Close()
	}()

	// PTY → WebSocket
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("pty read: %v", err)
				}
				return
			}
		}
	}()

	// WebSocket → PTY (with resize support)
	go func() {
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
					log.Printf("ws read: %v", err)
				}
				// Signal the PTY reader to stop by closing ptmx
				ptmx.Close()
				return
			}

			if msgType == websocket.TextMessage {
				var resize resizeMsg
				if json.Unmarshal(data, &resize) == nil && resize.Cols > 0 && resize.Rows > 0 {
					pty.Setsize(ptmx, &pty.Winsize{Cols: resize.Cols, Rows: resize.Rows})
					continue
				}
			}

			if _, err := ptmx.Write(data); err != nil {
				log.Printf("pty write: %v", err)
				return
			}
		}
	}()

	<-done
}
