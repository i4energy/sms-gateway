package modem

import "errors"

var (
	// ErrNilContext is returned when a nil context is passed to a function
	// that requires a valid context.
	//
	// This indicates a programming error. All functions that accept a context
	// parameter require a non-nil context, even if it's context.Background().
	ErrNilContext = errors.New("context is nil")

	// ErrMissingPort is returned when attempting to dial a serial connection
	// without specifying a port name.
	//
	// This indicates a configuration error. The PortName field must be set
	// to a valid device path (e.g., "/dev/ttyUSB0", "COM3") before dialing.
	ErrMissingPort = errors.New("missing required serial port name")

	// ErrPortOpenFail is returned when the underlying serial port cannot be
	// opened.
	//
	// This typically indicates a hardware issue (device not connected),
	// permission problem (insufficient access rights), or that another
	// process is already using the port. The wrapped error provides the
	// specific failure reason.
	ErrPortOpenFail = errors.New("failed to open serial port")
)
