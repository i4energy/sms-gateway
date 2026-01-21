package modem

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.bug.st/serial"
)

// Transport represents an established, bidirectional byte stream to a GSM modem.
//
// A Transport is assumed to be already connected and ready for use. It provides
// the low-level I/O primitives required to send AT commands and receive responses.
// Typical implementations include serial ports, TCP connections to emulators,
// or in-memory fakes used for testing.
type Transport interface {
	io.ReadWriteCloser
}

// Dialer opens a Transport to a GSM modem.
//
// Dialer abstracts how the modem connection is created (for example, via a
// serial port, TCP-based emulator, or test double) and is intended to be used
// during modem construction only. Once a Transport is obtained, the Dialer is
// no longer needed.
type Dialer interface {
	// Dial is responsible for creating and returning a connected Transport. It may
	// perform blocking operations and should respect cancellation and deadlines
	// provided by the context. Dial returns an error if the transport cannot be
	// established.
	Dial() (Transport, error)
}

// SerialDialer opens a GSM modem over a serial port using go.bug.st/serial.
//
// The returned serial.Port implements io.ReadWriteCloser and therefore satisfies
// the Transport interface. :contentReference[oaicite:1]{index=1}
type SerialDialer struct {
	// PortName is the OS device path (e.g. "/dev/ttyUSB0", "COM3").
	PortName string

	// Mode configures the serial port (baud, parity, etc.). If nil, the library
	// defaults are used (commonly 9600 8N1). :contentReference[oaicite:2]{index=2}
	Mode *serial.Mode
}

// Dial opens the serial port. If ctx is canceled before the open completes,
// Dial returns ctx.Err(). If the port opens concurrently with cancellation,
// the port is closed before returning.
func (d SerialDialer) Dial(ctx context.Context) (Transport, error) {
	if d.PortName == "" {
		return nil, fmt.Errorf("gsm: serial port name is required")
	}
	if ctx == nil {
		return nil, errors.New("gsm: context is nil")
	}

	type result struct {
		p   serial.Port
		err error
	}

	ch := make(chan result, 1)

	// serial.Open does not accept a context, so we run it in a goroutine
	// and race it against ctx cancellation.
	go func() {
		p, err := serial.Open(d.PortName, d.Mode) // :contentReference[oaicite:3]{index=3}
		ch <- result{p: p, err: err}
	}()

	select {
	case <-ctx.Done():
		// If the open eventually succeeds, close it to avoid leaking the fd.
		go func() {
			r := <-ch
			if r.err == nil && r.p != nil {
				_ = r.p.Close()
			}
		}()
		return nil, ctx.Err()

	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("open serial port %q: %w", d.PortName, r.err)
		}
		return r.p, nil
	}
}
