// Package testsupport contains a minimal local protocol fixture, not an NSQ
// broker. It only records the real driver's IDENTIFY fields and accepts NOP;
// its success never proves deployed topology, delivery or writer fencing.
package testsupport

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type NSQIdentify struct {
	ClientID string `json:"client_id"`
	Hostname string `json:"hostname"`
}

func NSQPublisherFixture(t *testing.T) (string, <-chan NSQIdentify) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	identities := make(chan NSQIdentify, 8)
	var mu sync.Mutex
	connections := map[net.Conn]bool{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections[conn] = true
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { _ = conn.Close(); mu.Lock(); delete(connections, conn); mu.Unlock() }()
				if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
					return
				}
				reader := bufio.NewReaderSize(conn, 1024)
				magic := make([]byte, 4)
				if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != "  V2" {
					return
				}
				command, err := reader.ReadString('\n')
				if err != nil || command != "IDENTIFY\n" {
					return
				}
				var size int32
				if binary.Read(reader, binary.BigEndian, &size) != nil || size < 1 || size > 64*1024 {
					return
				}
				body := make([]byte, size)
				if _, err := io.ReadFull(reader, body); err != nil {
					return
				}
				var identity NSQIdentify
				if json.Unmarshal(body, &identity) != nil || identity.ClientID == "" || identity.Hostname == "" {
					return
				}
				select {
				case identities <- identity:
				default:
					return
				}
				var frame bytes.Buffer
				_ = binary.Write(&frame, binary.BigEndian, int32(6))
				_ = binary.Write(&frame, binary.BigEndian, int32(0))
				frame.WriteString("OK")
				if _, err := io.Copy(conn, &frame); err != nil {
					return
				}
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if line != "NOP\n" {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		mu.Lock()
		for conn := range connections {
			_ = conn.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return listener.Addr().String(), identities
}

func ReceiveNSQIdentify(t *testing.T, identities <-chan NSQIdentify) NSQIdentify {
	t.Helper()
	select {
	case identity := <-identities:
		return identity
	case <-time.After(5 * time.Second):
		t.Fatal("local driver IDENTIFY was not observed")
		return NSQIdentify{}
	}
}
