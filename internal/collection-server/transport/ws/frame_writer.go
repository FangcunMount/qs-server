package ws

import (
	"context"
	"sync"

	"github.com/coder/websocket"
)

type frameWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func newFrameWriter(conn *websocket.Conn) *frameWriter {
	return &frameWriter{conn: conn}
}

func (w *frameWriter) write(ctx context.Context, frame outboundFrame) error {
	payload, err := encodeFrame(frame)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.Write(ctx, websocket.MessageText, payload)
}
