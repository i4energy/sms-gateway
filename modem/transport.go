package modem

import (
	"io"
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

// TODO: Add SerialDialer implementation when go.bug.st/serial package is available
// SerialDialer would open a GSM modem over a serial port using go.bug.st/serial
