package modem

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockTransport is a test transport that auto-responds to AT commands
type mockTransport struct {
	mu           sync.Mutex
	readBuf      *bytes.Buffer
	writeBuf     *bytes.Buffer
	closed       bool
	respondFunc  func(cmd string) // Custom response function
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		readBuf:  &bytes.Buffer{},
		writeBuf: &bytes.Buffer{},
	}
}

func (m *mockTransport) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, io.EOF
	}

	// If nothing in read buffer, wait a bit and check again
	// This simulates the modem not having data immediately
	if m.readBuf.Len() == 0 {
		return 0, io.EOF
	}

	return m.readBuf.Read(p)
}

func (m *mockTransport) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, io.ErrClosedPipe
	}

	// Record what was written
	n, err = m.writeBuf.Write(p)
	if err != nil {
		return n, err
	}

	// Auto-respond to commands
	cmd := string(p)
	if m.respondFunc != nil {
		m.respondFunc(cmd)
	} else {
		m.autoRespond(cmd)
	}

	return n, nil
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// autoRespond generates automatic responses for common AT commands
func (m *mockTransport) autoRespond(cmd string) {
	cmd = strings.TrimSpace(cmd)

	switch {
	case cmd == "AT":
		m.readBuf.WriteString("OK\r\n")

	case cmd == "ATE0":
		m.readBuf.WriteString("OK\r\n")

	case cmd == "ATE1":
		// Echo the command back first
		m.readBuf.WriteString("ATE1\r\n")
		m.readBuf.WriteString("OK\r\n")

	case cmd == "AT+CMEE=2":
		m.readBuf.WriteString("OK\r\n")

	case cmd == "AT+CPIN?":
		m.readBuf.WriteString("+CPIN: READY\r\n")
		m.readBuf.WriteString("OK\r\n")

	case cmd == "AT+CMGF=1":
		m.readBuf.WriteString("OK\r\n")

	case cmd == "AT+CSQ":
		m.readBuf.WriteString("+CSQ: 25,99\r\n")
		m.readBuf.WriteString("OK\r\n")

	case strings.HasPrefix(cmd, "AT+CPIN="):
		// PIN entry response
		m.readBuf.WriteString("OK\r\n")

	default:
		// Unknown command - return ERROR
		m.readBuf.WriteString("ERROR\r\n")
	}
}

// mockTransportWithPIN requires PIN entry
type mockTransportWithPIN struct {
	*mockTransport
	pinEntered bool
}

func newMockTransportWithPIN() *mockTransportWithPIN {
	mt := &mockTransportWithPIN{
		mockTransport: newMockTransport(),
		pinEntered:    false,
	}

	// Set up custom response function that handles PIN
	mt.respondFunc = func(cmd string) {
		cmd = strings.TrimSpace(cmd)

		switch {
		case cmd == "AT+CPIN?":
			if !mt.pinEntered {
				mt.readBuf.WriteString("+CPIN: SIM PIN\r\n")
				mt.readBuf.WriteString("OK\r\n")
			} else {
				mt.readBuf.WriteString("+CPIN: READY\r\n")
				mt.readBuf.WriteString("OK\r\n")
			}

		case strings.HasPrefix(cmd, "AT+CPIN="):
			// PIN entry
			mt.pinEntered = true
			mt.readBuf.WriteString("OK\r\n")

		default:
			mt.mockTransport.autoRespond(cmd)
		}
	}

	return mt
}

// mockDialer implements Dialer for testing
type mockDialer struct {
	transport Transport
	err       error
}

func (d mockDialer) Dial() (Transport, error) {
	return d.transport, d.err
}

func TestNew_Success(t *testing.T) {
	transport := newMockTransport()
	config := Config{
		Dialer:    mockDialer{transport: transport},
		ATTimeout: 1 * time.Second,
		EchoOn:    false,
	}

	ctx := context.Background()
	modem, err := New(ctx, config)

	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if modem == nil {
		t.Fatal("New() returned nil modem")
	}
	if modem.transport != transport {
		t.Error("modem transport not set correctly")
	}
	if modem.scanner == nil {
		t.Error("modem scanner not initialized")
	}
}

func TestNew_WithEchoOn(t *testing.T) {
	transport := newMockTransport()
	config := Config{
		Dialer:    mockDialer{transport: transport},
		ATTimeout: 1 * time.Second,
		EchoOn:    true,
	}

	ctx := context.Background()
	modem, err := New(ctx, config)

	if err != nil {
		t.Fatalf("New() with echo failed: %v", err)
	}
	if modem == nil {
		t.Fatal("New() returned nil modem")
	}
}

func TestNew_WithPIN(t *testing.T) {
	transport := newMockTransportWithPIN()
	config := Config{
		Dialer:    mockDialer{transport: transport},
		SimPIN:    "1234",
		ATTimeout: 1 * time.Second,
		EchoOn:    false,
	}

	ctx := context.Background()
	modem, err := New(ctx, config)

	if err != nil {
		t.Fatalf("New() with PIN failed: %v", err)
	}
	if modem == nil {
		t.Fatal("New() returned nil modem")
	}
	if !transport.pinEntered {
		t.Error("PIN was not entered during initialization")
	}
}

func TestNew_PINRequired(t *testing.T) {
	transport := newMockTransportWithPIN()
	config := Config{
		Dialer:    mockDialer{transport: transport},
		SimPIN:    "", // No PIN provided
		ATTimeout: 1 * time.Second,
		EchoOn:    false,
	}

	ctx := context.Background()
	modem, err := New(ctx, config)

	if err == nil {
		t.Fatal("New() should fail when PIN required but not provided")
	}
	if modem != nil {
		t.Error("New() should return nil modem on error")
	}
	if !strings.Contains(err.Error(), "SIM PIN required") {
		t.Errorf("expected SIM PIN required error, got: %v", err)
	}
}

func TestNew_NoDialer(t *testing.T) {
	config := Config{
		Dialer: nil,
	}

	ctx := context.Background()
	modem, err := New(ctx, config)

	if err != ErrNoDialer {
		t.Errorf("expected ErrNoDialer, got: %v", err)
	}
	if modem != nil {
		t.Error("New() should return nil modem when no dialer")
	}
}

func TestExec_SimpleCommand(t *testing.T) {
	transport := newMockTransport()
	config := Config{
		Dialer:    mockDialer{transport: transport},
		ATTimeout: 1 * time.Second,
		EchoOn:    false,
	}

	ctx := context.Background()
	modem, err := New(ctx, config)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Test a simple query
	resp, err := modem.exec(ctx, "AT+CSQ")
	if err != nil {
		t.Fatalf("exec(AT+CSQ) failed: %v", err)
	}

	if !strings.Contains(resp, "+CSQ: 25,99") {
		t.Errorf("expected response to contain signal quality, got: %q", resp)
	}
	if !strings.Contains(resp, "OK") {
		t.Errorf("expected response to contain OK, got: %q", resp)
	}
}

func TestExec_WithTimeout(t *testing.T) {
	// Create a transport that never responds
	transport := newMockTransport()

	// Override respondFunc to do nothing (no response)
	transport.respondFunc = func(cmd string) {
		// Don't write anything - simulate timeout
	}

	config := Config{
		Dialer:    mockDialer{transport: transport},
		ATTimeout: 100 * time.Millisecond,
		EchoOn:    false,
	}

	ctx := context.Background()
	modem, err := New(ctx, config)
	if err != nil {
		// Init will fail because transport doesn't respond
		// This is expected
		return
	}

	// If we somehow got here, test exec timeout
	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = modem.exec(ctx2, "AT")
	if err == nil {
		t.Error("exec() should timeout when no response")
	}
}

func TestClassifyIntegration(t *testing.T) {
	// Test that exec properly uses at.Classify
	transport := newMockTransport()

	// Set custom response function that sends URC mixed with response
	transport.respondFunc = func(cmd string) {
		if strings.TrimSpace(cmd) == "AT+CSQ" {
			// Send a URC first (should be ignored)
			transport.readBuf.WriteString("+CMTI: \"SM\",5\r\n")
			// Then the actual response
			transport.readBuf.WriteString("+CSQ: 25,99\r\n")
			transport.readBuf.WriteString("OK\r\n")
		} else {
			// Use default responses for other commands
			transport.autoRespond(cmd)
		}
	}

	config := Config{
		Dialer:    mockDialer{transport: transport},
		ATTimeout: 1 * time.Second,
		EchoOn:    false,
	}

	ctx := context.Background()
	modem, err := New(ctx, config)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	resp, err := modem.exec(ctx, "AT+CSQ")
	if err != nil {
		t.Fatalf("exec() failed: %v", err)
	}

	// URC should not be in response
	if strings.Contains(resp, "+CMTI") {
		t.Errorf("URC should be filtered out, got: %q", resp)
	}

	// Data should be in response
	if !strings.Contains(resp, "+CSQ: 25,99") {
		t.Errorf("expected data line in response, got: %q", resp)
	}
}
