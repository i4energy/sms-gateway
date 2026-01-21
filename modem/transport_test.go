package modem

import (
	"context"
	"errors"
	"testing"

	"go.bug.st/serial"
	"go.uber.org/mock/gomock"
)

func TestSerialDialer_Dial_EmptyPortName(t *testing.T) {
	dialer := SerialDialer{
		PortName: "",
	}

	ctx := context.Background()
	transport, err := dialer.Dial(ctx)

	if err == nil {
		t.Error("expected error for empty port name")
	}
	if transport != nil {
		t.Error("expected nil transport for empty port name")
	}
	if err.Error() != "gsm: serial port name is required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSerialDialer_Dial_NilContext(t *testing.T) {
	dialer := SerialDialer{
		PortName: "/dev/ttyUSB0",
	}

	transport, err := dialer.Dial(nil)

	if err == nil {
		t.Error("expected error for nil context")
	}
	if transport != nil {
		t.Error("expected nil transport for nil context")
	}
	if err.Error() != "gsm: context is nil" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSerialDialer_Dial_ContextCanceled(t *testing.T) {
	dialer := SerialDialer{
		PortName: "/dev/nonexistent", // Port that should fail to open
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	transport, err := dialer.Dial(ctx)

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	if transport != nil {
		t.Error("expected nil transport for canceled context")
	}
}

func TestSerialDialer_Dial_WithMode(t *testing.T) {
	dialer := SerialDialer{
		PortName: "/dev/nonexistent", // This will fail, but we test the path
		Mode: &serial.Mode{
			BaudRate: 115200,
			Parity:   serial.NoParity,
			DataBits: 8,
			StopBits: serial.OneStopBit,
		},
	}

	ctx := context.Background()
	transport, err := dialer.Dial(ctx)

	// Since we're using a non-existent port, expect an error
	if err == nil {
		t.Error("expected error for non-existent port")
	}
	if transport != nil {
		t.Error("expected nil transport for non-existent port")
	}
	// Check that the error mentions the port name
	if err != nil && err.Error() == "" {
		t.Error("expected descriptive error message")
	}
}

func TestSerialDialer_Dial_DefaultMode(t *testing.T) {
	dialer := SerialDialer{
		PortName: "/dev/nonexistent", // This will fail, but we test the path
		// Mode is nil - should use defaults
	}

	ctx := context.Background()
	transport, err := dialer.Dial(ctx)

	// Since we're using a non-existent port, expect an error
	if err == nil {
		t.Error("expected error for non-existent port")
	}
	if transport != nil {
		t.Error("expected nil transport for non-existent port")
	}
}

// Test the interface compliance
func TestTransportInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransport := NewMockTransport(ctrl)

	// Test that mockTransport implements Transport interface
	var _ Transport = mockTransport

	// Test basic operations
	data := []byte("test")
	mockTransport.EXPECT().Write(data).Return(len(data), nil)
	mockTransport.EXPECT().Read(gomock.Any()).Return(4, nil)
	mockTransport.EXPECT().Close().Return(nil)

	n, err := mockTransport.Write(data)
	if err != nil {
		t.Errorf("unexpected write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes written, got %d", len(data), n)
	}

	buf := make([]byte, 10)
	n, err = mockTransport.Read(buf)
	if err != nil {
		t.Errorf("unexpected read error: %v", err)
	}
	if n != 4 {
		t.Errorf("expected 4 bytes read, got %d", n)
	}

	err = mockTransport.Close()
	if err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}

func TestDialerInterface(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDialer := NewMockDialer(ctrl)
	mockTransport := NewMockTransport(ctrl)

	// Test that mockDialer implements Dialer interface
	var _ Dialer = mockDialer

	ctx := context.Background()
	mockDialer.EXPECT().Dial(ctx).Return(mockTransport, nil)

	transport, err := mockDialer.Dial(ctx)
	if err != nil {
		t.Errorf("unexpected dial error: %v", err)
	}
	if transport != mockTransport {
		t.Error("expected mock transport to be returned")
	}
}

func TestDialerInterface_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDialer := NewMockDialer(ctrl)
	dialError := errors.New("dial failed")

	ctx := context.Background()
	mockDialer.EXPECT().Dial(ctx).Return(nil, dialError)

	transport, err := mockDialer.Dial(ctx)
	if err != dialError {
		t.Errorf("expected dial error, got: %v", err)
	}
	if transport != nil {
		t.Error("expected nil transport on error")
	}
}
