package modem

import (
	"context"
	"errors"
	"testing"
)

func TestSerialDialerErrors(t *testing.T) {
	t.Run("empty port name", func(t *testing.T) {
		dialer := SerialDialer{
			PortName: "",
		}

		ctx := context.Background()
		transport, err := dialer.Dial(ctx)

		if !errors.Is(err, ErrMissingPort) {
			t.Errorf("expected ErrMissingPort error for empty port name, got %v", err)
		}
		if transport != nil {
			t.Error("expected nil transport for empty port name")
		}
	})

	t.Run("non-existent port", func(t *testing.T) {
		dialer := SerialDialer{
			PortName: "/dev/nonexistent", // Port that should fail to open
		}

		ctx := context.Background()
		transport, err := dialer.Dial(ctx)

		if !errors.Is(err, ErrPortOpenFail) {
			t.Errorf("expected ErrPortOpenFail error for non-existent port, got %v", err)
		}
		if transport != nil {
			t.Error("expected nil transport for non-existent port")
		}
	})

	t.Run("nil context", func(t *testing.T) {
		dialer := SerialDialer{
			PortName: "/dev/ttyUSB0",
		}

		transport, err := dialer.Dial(nil)

		if !errors.Is(err, ErrNilContext) {
			t.Errorf("expected ErrNilContext for nil context, got %v", err)
		}
		if transport != nil {
			t.Error("expected nil transport for nil context")
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		dialer := SerialDialer{
			PortName: "/dev/nonexistent", // Port that should fail to open
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		transport, err := dialer.Dial(ctx)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
		if transport != nil {
			t.Error("expected nil transport for canceled context")
		}
	})
}
