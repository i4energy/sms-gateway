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

	// ErrLineTooLong is returned when a modem response line exceeds the
	// maximum allowed length.
	//
	// This typically indicates malformed input, unexpected binary data,
	// or a protocol framing error.
	ErrLineTooLong = errors.New("response line too long")
)
