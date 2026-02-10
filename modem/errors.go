package modem

import "errors"

var (
	// ErrNoDialer is returned when a Modem is constructed without a Dialer.
	//
	// This indicates a configuration error. A Dialer is required in order to
	// establish a connection to the modem.
	ErrNoDialer = errors.New("no dialer configured")

	// ErrNotInitialized is returned when an operation is attempted on a Modem
	// that has not been successfully initialized.
	//
	// This can occur if initialization failed or if the Modem was not created
	// via NewModem.
	ErrNotInitialized = errors.New("modem not initialized")

	// ErrAlreadyClosed is returned when Close is called on a Modem that has
	// already been closed.
	ErrAlreadyClosed = errors.New("modem already closed")

	// ErrSIMPinRequired is returned when the SIM card requires a PIN and no
	// PIN was provided in the Config.
	//
	// Callers may handle this error specially (for example, by prompting
	// the user for a PIN) and retry initialization.
	ErrSIMPinRequired = errors.New("SIM PIN required")

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

	// ErrLoopRunning is returned when Loop() is called while the modem loop is
	// already running. This is used to prohibit concurrent execution of multiple
	// loops, which could cause race conditions and undefined behavior.
	ErrLoopRunning = errors.New("modem loop already running")
)
