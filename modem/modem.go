package modem

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"i4.energy/across/smsgw/at"
)

// Modem represents a GSM/3G/4G cellular modem that communicates via AT commands.
// It provides thread-safe access to SMS functionality and modem operations through
// a centralized event loop that handles all transport I/O.
type Modem struct {
	// transport provides the physical connection to the modem (serial, TCP, etc.)
	transport Transport
	// config contains the modem configuration settings
	config Config
	// closed indicates if the modem has been shut down
	closed bool
	// loopRunning indicates if the Loop is currently running
	loopRunning bool
	// atTimeout is the default timeout for AT command responses
	atTimeout time.Duration
	// simPIN is the SIM card PIN code for authentication
	simPIN string

	// Communication channels for Loop coordination
	// urcChan receives Unsolicited Result Codes from the modem
	urcChan chan string
	// commands queues AT command requests for the Loop to process
	commands chan *commandRequest

	// Loop control
	// loopCtx controls the lifecycle of the main event loop
	loopCtx context.Context
	// loopCancel cancels the main event loop
	loopCancel context.CancelFunc
}

// commandRequest represents an AT command request to be executed by the Loop.
// It contains the command string, response channel, and execution context.
type commandRequest struct {
	// cmd is the AT command string to send to the modem
	cmd string
	// respChan receives the command response from the Loop
	respChan chan commandResponse
	// ctx provides timeout and cancellation control for the command
	ctx context.Context
}

// commandResponse contains the result of an AT command execution.
// It includes both the response data and any error that occurred.
type commandResponse struct {
	// response contains the complete response text from the modem
	response string
	// err contains any error that occurred during command execution
	err error
}

// PollConfig defines configuration for polling operations like waiting for SIM readiness.
type PollConfig struct {
	// Interval is the time between polling attempts
	Interval time.Duration
	// Timeout is the maximum time to wait for the condition
	Timeout time.Duration
	// MaxRetries is the maximum number of polling attempts
	MaxRetries int
}

// New creates a new Modem instance with the given configuration.
// It establishes the transport connection, initializes the modem
// hardware with common actions and prepares the event loop context.
//
// Returns an error if the transport connection or modem initialization
// fails.
func New(ctx context.Context, config Config) (*Modem, error) {
	if config.dialer == nil {
		return nil, ErrNoDialer
	}
	transport, err := config.dialer.Dial(ctx)
	if err != nil {
		return nil, err
	}

	m := &Modem{
		atTimeout: config.atTimeout,
		simPIN:    config.simPIN,
		transport: transport,
		urcChan:   make(chan string, 100), // Buffered to prevent blocking on URCs
		// No queue for commands
		commands: make(chan *commandRequest),
	}

	// Prepare context for Loop (but don't start it yet)
	m.loopCtx, m.loopCancel = context.WithCancel(ctx)

	// Initialize the modem with proper timeout
	initCtx := ctx
	if config.initTimeout > 0 {
		var cancel context.CancelFunc
		initCtx, cancel = context.WithTimeout(ctx, config.initTimeout)
		defer cancel()
	}

	if err := m.init(initCtx); err != nil {
		if m.transport != nil {
			transport.Close()
		}
		return nil, fmt.Errorf("initialize modem: %w", err)
	}

	return m, nil
}

// Loop is the main event loop that handles all transport I/O operations.
// It must be called exactly once after New() and before any other modem operations.
// The Loop coordinates all communication with the modem hardware:
//
// 1. Processes command requests from exec() calls
// 2. Writes AT commands to the transport
// 3. Reads and parses responses from the transport
// 4. Dispatches URCs (Unsolicited Result Codes) to subscribers
// 5. Returns command responses to waiting exec() calls
//
// The Loop runs until the provided context is cancelled. It's the ONLY goroutine
// that reads from the transport, preventing race conditions and ensuring URCs
// are never lost.
//
// Usage:
//
//	modem, err := New(ctx, config)
//	if err != nil { return err }
//
//	// Start the loop (typically in a goroutine)
//	go modem.Loop(ctx)
//
//	// Now exec() calls will work
//	resp, err := modem.exec(ctx, "AT")
func (m *Modem) Loop(ctx context.Context) error {
	if m.loopRunning {
		return ErrLoopRunning
	}
	m.loopRunning = true
	defer func() {
		m.loopRunning = false
	}()
	scanner := bufio.NewScanner(m.transport)
	scanner.Split(at.Splitter)

	// Channels for tokens and errors from the scanner goroutine
	tokens := make(chan string, 10)
	scanErrs := make(chan error, 1)

	// Start goroutine to read tokens from transport
	go func() {
		defer func() {
			close(tokens)
		}()
		for scanner.Scan() {
			token := scanner.Text()
			if token != "" {
				select {
				case tokens <- token:
				case <-ctx.Done():
					return
				}
			}
		}
		// Scanner stopped - check if there was an error
		if err := scanner.Err(); err != nil {
			select {
			case scanErrs <- err:
			case <-ctx.Done():
			}
		}
	}()

	// Current command being processed
	var currentCmd *commandRequest
	var currentLines []string

	for {
		select {
		case <-ctx.Done():
			// Context cancelled - shut down gracefully
			if currentCmd != nil {
				currentCmd.respChan <- commandResponse{err: ctx.Err()}
			}
			return ctx.Err()

		case req := <-m.commands:
			currentCmd = req
			currentLines = nil

			// Write the AT command to the transport
			wire := strings.TrimSpace(req.cmd) + "\r"
			if _, err := m.transport.Write([]byte(wire)); err != nil {
				req.respChan <- commandResponse{err: fmt.Errorf("write command %q: %w", req.cmd, err)}
				currentCmd = nil
				continue
			}

		case token, ok := <-tokens:
			if !ok {

				// Token channel closed - scanner stopped
				if currentCmd != nil {
					currentCmd.respChan <- commandResponse{response: token, err: io.EOF}
					currentCmd = nil
					currentLines = nil
				}
				return io.EOF
			}

			// Classify the token to determine how to handle it
			respType := at.Classify(token)

			switch respType {
			case at.TypeURC:
				// Unsolicited Result Code - always dispatch to URC channel
				// URCs can arrive at any time, even during command execution
				select {
				case m.urcChan <- token:
					// URC dispatched successfully
				default:
					// URC channel is full - drop the URC
					// In production, you might want to log this
				}

			case at.TypeFinal:
				// Final response (OK, ERROR, +CME ERROR, etc.)
				if currentCmd != nil {
					currentLines = append(currentLines, token)
					response := strings.Join(currentLines, "\n")

					if token == at.OK {
						// Command succeeded
						currentCmd.respChan <- commandResponse{response: response}
					} else {
						// Command failed (ERROR, +CME ERROR, etc.)
						currentCmd.respChan <- commandResponse{response: response, err: errors.New(token)}
					}

					currentCmd = nil
					currentLines = nil
				}
				// If no current command, ignore the final response (orphaned)

			case at.TypeData:
				// Intermediate data response (e.g., +CSQ: 15,99)
				if currentCmd != nil {
					currentLines = append(currentLines, token)
				}
				// If no current command, ignore the data (orphaned)

			case at.TypePrompt:
				// SMS prompt (">") - return immediately for SMS text input
				if currentCmd != nil {
					currentLines = append(currentLines, token)
					response := strings.Join(currentLines, "\n")
					currentCmd.respChan <- commandResponse{response: response}
					currentCmd = nil
					currentLines = nil
				}
			}

			// Check if current command has timed out
			if currentCmd != nil {
				select {
				case <-currentCmd.ctx.Done():
					// Command timed out or was cancelled
					currentCmd.respChan <- commandResponse{err: fmt.Errorf("command timeout: %w", currentCmd.ctx.Err())}
					currentCmd = nil
					currentLines = nil
				default:
					// Command still within timeout
				}
			}

		case err := <-scanErrs:
			// Scanner error - notify current command if any
			if currentCmd != nil {
				currentCmd.respChan <- commandResponse{err: fmt.Errorf("read error: %w", err)}
				currentCmd = nil
				currentLines = nil
			}
			return fmt.Errorf("scanner error: %w", err)
		}
	}
}

// URC returns a read-only channel that receives Unsolicited Result Codes.
// These are asynchronous notifications from the modem (e.g., incoming SMS,
// network status changes, etc.). The channel is buffered, but may drop
// some URC if not consumed fast enough.
func (m *Modem) URC() <-chan string {
	return m.urcChan
}

// Close shuts down the modem and releases all resources.
// It stops the event loop, closes the transport connection, and marks
// the modem as closed. After calling Close(), the modem cannot be reused.
func (m *Modem) Close() error {

	if m.closed {
		return ErrAlreadyClosed
	}

	m.closed = true

	// Stop the Loop if it's running
	if m.loopCancel != nil {
		m.loopCancel()
	}

	if m.transport != nil {
		return m.transport.Close()
	}

	return nil
}

// init performs the initial setup sequence for the modem hardware.
// This method is called during New() and must complete successfully
// before the modem can be used.
func (m *Modem) init(ctx context.Context) error {
	// 1. Wake-up / sanity check
	if err := m.expectOkDirect(ctx, at.CmdAt); err != nil {
		return fmt.Errorf("modem not responding: %w", err)
	}

	if err := m.expectOkDirect(ctx, at.CmdEchoOff); err != nil {
		return fmt.Errorf("could not disable echo: %w", err)
	}

	if err := m.expectOkDirect(ctx, at.CmdVerboseErrors); err != nil {
		return fmt.Errorf("could not enable verbose errors: %w", err)
	}

	// 4. Check SIM status
	simStatus, err := m.execDirect(ctx, at.CmdSimStatus)
	if err != nil {
		return fmt.Errorf("query SIM status: %w", err)
	}

	switch {
	case strings.Contains(simStatus, at.SimReady):
		// OK

	case strings.Contains(simStatus, at.SimPin):
		if m.simPIN == "" {
			return ErrSIMPinRequired
		}
		if err := m.expectOkDirect(ctx, fmt.Sprintf(`AT+CPIN="%s"`, m.simPIN)); err != nil {
			return fmt.Errorf("enter SIM PIN: %w", err)
		}

		// Wait until SIM becomes ready
		if err := m.waitForSIMReady(ctx, PollConfig{}); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported SIM state: %q", simStatus)
	}

	// 5. Select SMS text mode
	if err := m.expectOkDirect(ctx, at.CmdSetTextMode); err != nil {
		return fmt.Errorf("set SMS text mode: %w", err)
	}

	return nil
}

// exec sends an AT command to the modem and waits for the response.
// This method coordinates with the Loop() to ensure thread-safe command execution.
// The Loop() must be running before calling this method.
func (m *Modem) exec(ctx context.Context, cmd string) (string, error) {
	if m.closed {
		return "", ErrAlreadyClosed
	}

	if m.transport == nil {
		return "", ErrNotInitialized
	}

	// Apply per-command timeout if context has none
	if _, ok := ctx.Deadline(); !ok && m.config.atTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.config.atTimeout)
		defer cancel()
	}

	// Create command request
	req := &commandRequest{
		cmd:      cmd,
		respChan: make(chan commandResponse, 1), // Buffered to prevent blocking
		ctx:      ctx,
	}

	// Send request to Loop
	select {
	case m.commands <- req:
		// Request queued successfully
	case <-ctx.Done():
		return "", fmt.Errorf("command cancelled before sending: %w", ctx.Err())
	}

	// Wait for response from Loop
	select {
	case resp := <-req.respChan:
		return resp.response, resp.err
	case <-ctx.Done():
		return "", fmt.Errorf("command timeout: %w", ctx.Err())
	}
}

// execDirect executes an AT command directly on the transport without
// using the channel mechanism and handles the complete request-response
// cycle including timeout management. It is used during modem initialization
// when not yet accepting commands.
//
// WARNING: This method should only be used during initialization.
// Use exec() for normal operations.
func (m *Modem) execDirect(ctx context.Context, cmd string) (string, error) {
	if m.closed {
		return "", ErrAlreadyClosed
	}
	if m.transport == nil {
		return "", ErrNotInitialized
	}

	if _, ok := ctx.Deadline(); !ok && m.atTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.atTimeout)
		defer cancel()
	}

	wire := strings.TrimSpace(cmd) + "\r"
	if _, err := m.transport.Write([]byte(wire)); err != nil {
		return "", fmt.Errorf("write command %q: %w", cmd, err)
	}

	scanner := bufio.NewScanner(m.transport)
	scanner.Split(at.Splitter)

	var lines []string

	for {
		select {
		case <-ctx.Done():
			return strings.Join(lines, "\n"), ctx.Err()
		default:
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return strings.Join(lines, "\n"), fmt.Errorf("read error: %w", err)
			}
			return strings.Join(lines, "\n"), io.EOF
		}

		token := scanner.Text()
		if token == "" {
			continue
		}

		respType := at.Classify(token)

		switch respType {
		case at.TypeFinal:
			lines = append(lines, token)

			response := strings.Join(lines, "\n")
			if token == at.OK {
				return response, nil
			} else {
				return response, errors.New(token)
			}

		case at.TypeData:
			lines = append(lines, token)

		case at.TypeURC:
			// Ignore URCs in direct exec
			continue
		case at.TypePrompt:
			lines = append(lines, token)
			response := strings.Join(lines, "\n")
			return response, nil
		}
	}
}

// expectOkDirect executes an AT command and validates that the response
// contains "OK". This is a convenience method for commands that should
// succeed with a simple OK response.
//
// Used during initialization for basic configuration commands.
func (m *Modem) expectOkDirect(ctx context.Context, cmd string) error {
	resp, err := m.execDirect(ctx, cmd)
	if err != nil {
		return err
	}
	if !strings.Contains(resp, at.OK) {
		return fmt.Errorf("unexpected response: %q", resp)
	}
	return nil
}

// waitForSIMReady polls the SIM card status until it reports ready state.
// This is necessary after entering a SIM PIN, as the SIM card needs time
// to authenticate and become operational. Uses configurable polling interval
// and retry limits to avoid infinite waiting.
func (m *Modem) waitForSIMReady(ctx context.Context, config PollConfig) error {
	var (
		pollInterval = config.Interval
		timeout      = config.Timeout
		maxRetries   = config.MaxRetries
	)

	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if maxRetries <= 0 {
		maxRetries = int(timeout / pollInterval)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	retries := 0

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("SIM not ready: %w", ctx.Err())
		case <-ticker.C:
			retries++
			if retries > maxRetries {
				return fmt.Errorf("SIM not ready after %d retries", maxRetries)
			}
			resp, err := m.execDirect(ctx, at.CmdSimStatus)
			if err != nil {
				// Fail fast on critical errors
				if errors.Is(err, ErrAlreadyClosed) || errors.Is(err, ErrNotInitialized) {
					return fmt.Errorf("SIM status check failed: %w", err)
				}
				continue
			}
			if strings.Contains(resp, at.SimReady) {
				return nil
			}
		}
	}
}
