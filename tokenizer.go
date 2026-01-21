package gsm

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"time"
)

type Modem struct {
	reader *bufio.Reader

	promptChan chan struct{}
	urcChan    chan string
	respChan   chan string
	dataChan   chan string
	errChan    chan error

	smsChan    chan SMS
	statusChan chan Event
	callChan   chan Call
}

// SMS represents a parsed Short Message.
type SMS struct {
	Index    int       // Storage index (from +CMTI)
	Sender   string    // Originating address
	Text     string    // Message body
	Resource string    // Storage type (e.g., "SM" for SIM, "ME" for Phone)
	Time     time.Time // Timestamp from the Service Center
}

// Call represents an incoming voice or data call.
type Call struct {
	Number string    // Phone number of the caller (if Caller ID is enabled)
	Type   string    // Type of address (e.g., International, National)
	Active bool      // Status of the call (True for RING, False for Hangup/No Carrier)
	Time   time.Time // Arrival time
}

var (
	ErrNilReader = errors.New("reader cannot be nil")
)

func NewModem(r io.Reader) (*Modem, error) {
	if r == nil {
		return nil, ErrNilReader
	}

	m := &Modem{
		reader:     bufio.NewReaderSize(r, 16*1024),
		promptChan: make(chan struct{}),
		respChan:   make(chan string),
		dataChan:   make(chan string),
		urcChan:    make(chan string, 10),
		errChan:    make(chan error, 10),
		smsChan:    make(chan SMS, 10),
		statusChan: make(chan Event, 100),
		callChan:   make(chan Call, 10),
	}

	go m.scanLoop()

	return m, nil
}

const (
	// Terminal Control
	CRLF   = "\r\n"
	Prompt = "> "

	// Response Codes
	OK         = "OK"
	ERROR      = "ERROR"
	NoCarrier  = "NO CARRIER"
	NoDialtone = "NO DIALTONE"
	Busy       = "BUSY"
	NoAnswer   = "NO ANSWER"
	CmeError   = "+CME ERROR:"
	CmsError   = "+CMS ERROR:"

	// URCs (Unsolicited Result Codes)
	UrcNewMsg         = "+CMTI:"
	UrcMessageReport  = "+CDSI:"
	UrcSignalStrength = "+CSQ:"
	UrcCall           = "RING"
)

type EventType int

const (
	EvNetworkUpdate EventType = iota
	EvSignalUpdate
	EvSimStatus
	EvCallIncoming
)

type Event struct {
	Type  EventType
	Value string // e.g., "READY", "HOME", or signal "24"
	Raw   string // Always keep the raw line for debugging
}

type ResponseType int

const (
	TypeFinal  ResponseType = iota // OK, ERROR
	TypeURC                        // Asynchronous notifications
	TypeData                       // Intermediate command output (+CSQ: ...)
	TypePrompt                     // SMS input prompt
)

// ATSplitter handles the "No Echo" scenario.
// It splits by CRLF but also recognizes the SMS prompt.
func ATSplitter(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// 1. Match SMS Prompt
	if bytes.HasPrefix(data, []byte(Prompt)) {
		return len(Prompt), data[0:len(Prompt)], nil
	}

	// 2. Match standard line ending with CRLF
	if i := bytes.Index(data, []byte(CRLF)); i >= 0 {
		return i + len(CRLF), data[0:i], nil
	}

	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// Classify identifies the nature of the modem output
func classify(line string) ResponseType {
	if line == Prompt {
		return TypePrompt
	}

	// Direct matches for final results
	switch line {
	case OK, ERROR:
		return TypeFinal
	}

	// Prefix matches
	switch {
	case strings.HasPrefix(line, CmeError), strings.HasPrefix(line, CmsError):
		return TypeFinal
	case strings.HasPrefix(line, UrcNewMsg), line == UrcCall:
		return TypeURC
	default:
		return TypeData
	}
}

// listen manages the background reading and dispatching of modem tokens
func (m *Modem) scanLoop() {
	scanner := bufio.NewScanner(m.reader)
	// ATSplitter is the custom SplitFunc we wrote earlier
	scanner.Split(ATSplitter)

	for scanner.Scan() {
		// Get the token and clean up surrounding whitespace/newlines
		token := scanner.Text()

		// Note: We don't TrimSpace on the Prompt because "> " has a trailing space
		line := token
		if line != Prompt {
			line = strings.TrimSpace(token)
		}

		// Skip empty pulses from the modem
		if line == "" {
			continue
		}

		// Dispatch based on category
		switch classify(line) {
		case TypePrompt:
			// Signal that the modem is ready for SMS body data
			m.promptChan <- struct{}{}

		case TypeURC:
			// Send to the asynchronous URC handler (e.g., SMS/Call listener)
			// Using a select with default prevents a slow listener from blocking the modem
			select {
			case m.urcChan <- line:
			default:
				// Log or drop if URC buffer is full
			}

		case TypeFinal:
			// Signal that the current command is finished
			m.respChan <- line

		case TypeData:
			// This is intermediate data (like the signal strength value)
			m.dataChan <- line
		}
	}

	if err := scanner.Err(); err != nil {
		// Handle serial port read errors (e.g., device unplugged)
		m.errChan <- err
	}
}

// URC returns a read-only channel for incoming SMS notifications
func (m *Modem) URC() <-chan string {
	return m.urcChan
}

func (m *Modem) IncomingSMS() <-chan SMS {
	return m.smsChan
}

func (m *Modem) Status() <-chan Event {
	return m.statusChan
}

func (m *Modem) IncomingCall() <-chan Call {
	return m.callChan
}
func (m *Modem) Errors() <-chan error {
	return m.errChan
}

// Additional public accessors for testing
func (m *Modem) PromptChan() <-chan struct{} {
	return m.promptChan
}

func (m *Modem) RespChan() <-chan string {
	return m.respChan
}

func (m *Modem) DataChan() <-chan string {
	return m.dataChan
}
