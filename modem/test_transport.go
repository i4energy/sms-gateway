package modem

import (
	"io"
	"sync"
)

// TestTransport is a test helper that simulates a blocking transport using channels.
// This is needed because the Loop's scanner goroutine continuously reads from the transport,
// and we need reads to block until data is available (like a real serial port would).
type TestTransport struct {
	mu       sync.Mutex
	readChan chan []byte
	closed   bool
}

// NewTestTransport creates a new test transport for testing.
// Exported for use in tests.
func NewTestTransport() *TestTransport {
	return &TestTransport{
		readChan: make(chan []byte, 10),
	}
}

func (t *TestTransport) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (t *TestTransport) Read(p []byte) (n int, err error) {
	data, ok := <-t.readChan
	if !ok {
		return 0, io.EOF
	}
	return copy(p, data), nil
}

func (t *TestTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.readChan)
	return nil
}

// SendData queues data to be read by the transport.
// This simulates receiving data from the modem.
func (t *TestTransport) SendData(data string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.readChan <- []byte(data)
	}
}
