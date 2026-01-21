package modem

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"i4.energy/across/sms_gw/at"
)

type Modem struct {
	mu        sync.Mutex
	transport Transport
	config    Config
	scanner   *bufio.Scanner
}

func New(ctx context.Context, config Config) (*Modem, error) {
	config.setDefaults()
	if err := config.validate(); err != nil {
		return nil, err
	}

	transport, err := config.Dialer.Dial(ctx)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(transport)
	scanner.Split(at.Splitter)

	m := &Modem{
		config:    config,
		transport: transport,
		scanner:   scanner,
	}

	// Initialize the modem with proper timeout
	initCtx := ctx
	if m.config.InitTimeout > 0 {
		var cancel context.CancelFunc
		initCtx, cancel = context.WithTimeout(ctx, m.config.InitTimeout)
		defer cancel()
	}

	if err := m.init(initCtx); err != nil {
		transport.Close()
		return nil, fmt.Errorf("initialize modem: %w", err)
	}

	return m, nil
}

func (m *Modem) readToken() (string, error) {
	if !m.scanner.Scan() {
		if err := m.scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return strings.TrimSpace(m.scanner.Text()), nil
}

func (m *Modem) init(ctx context.Context) error {
	// 1. Wake-up / sanity check
	if err := m.expectOK(ctx, "AT"); err != nil {
		return fmt.Errorf("modem not responding: %w", err)
	}

	// 2. Echo handling
	if m.config.EchoOn {
		_ = m.expectOK(ctx, "ATE1") // best effort
	} else {
		if err := m.expectOK(ctx, "ATE0"); err != nil {
			return fmt.Errorf("disable echo: %w", err)
		}
	}

	// 3. Enable verbose errors (recommended)
	_ = m.expectOK(ctx, "AT+CMEE=2") // ignore failure (not all modems support it)

	// 4. Check SIM status
	simStatus, err := m.query(ctx, "AT+CPIN?")
	if err != nil {
		return fmt.Errorf("query SIM status: %w", err)
	}

	switch {
	case strings.Contains(simStatus, "READY"):
		// OK

	case strings.Contains(simStatus, "SIM PIN"):
		if m.config.SimPIN == "" {
			return ErrSIMPinRequired
		}
		if err := m.expectOK(ctx, fmt.Sprintf(`AT+CPIN="%s"`, m.config.SimPIN)); err != nil {
			return fmt.Errorf("enter SIM PIN: %w", err)
		}

		// Wait until SIM becomes ready
		if err := m.waitForSIMReady(ctx); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported SIM state: %q", simStatus)
	}

	// 5. Select SMS text mode
	if err := m.expectOK(ctx, "AT+CMGF=1"); err != nil {
		return fmt.Errorf("set SMS text mode: %w", err)
	}

	return nil
}

func (m *Modem) expectOK(ctx context.Context, cmd string) error {
	resp, err := m.exec(ctx, cmd)
	if err != nil {
		return err
	}
	if !strings.Contains(resp, "OK") {
		return fmt.Errorf("unexpected response: %q", resp)
	}
	return nil
}

func (m *Modem) query(ctx context.Context, cmd string) (string, error) {
	return m.exec(ctx, cmd)
}

func (m *Modem) waitForSIMReady(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("SIM not ready: %w", ctx.Err())
		case <-ticker.C:
			resp, err := m.exec(ctx, "AT+CPIN?")
			if err != nil {
				continue
			}
			if strings.Contains(resp, "READY") {
				return nil
			}
		}
	}
}

func (m *Modem) exec(ctx context.Context, cmd string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.transport == nil {
		return "", ErrNotInitialized
	}

	// Apply per-command timeout if context has none
	if _, ok := ctx.Deadline(); !ok && m.config.ATTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.config.ATTimeout)
		defer cancel()
	}

	// Set read deadline if supported
	if d, ok := m.transport.(interface {
		SetReadDeadline(time.Time) error
	}); ok {
		if deadline, ok := ctx.Deadline(); ok {
			_ = d.SetReadDeadline(deadline)
		}
	}

	// Write command
	wire := strings.TrimSpace(cmd) + "\r"
	if _, err := io.WriteString(m.transport, wire); err != nil {
		return "", fmt.Errorf("write command %q: %w", cmd, err)
	}

	var lines []string

	for {
		select {
		case <-ctx.Done():
			return strings.Join(lines, "\n"), ctx.Err()
		default:
		}

		token, err := m.readToken()
		if err != nil {
			return strings.Join(lines, "\n"), err
		}

		if token == "" {
			continue
		}

		// Ignore echoed command if echo is enabled
		if m.config.EchoOn && token == strings.TrimSpace(cmd) {
			continue
		}

		// Classify the response using at.Classify
		respType := at.Classify(token)

		switch respType {
		case at.TypeFinal:
			lines = append(lines, token)
			if token == at.OK {
				return strings.Join(lines, "\n"), nil
			}
			// ERROR, +CME ERROR, +CMS ERROR, etc.
			return strings.Join(lines, "\n"), errors.New(token)

		case at.TypeData:
			lines = append(lines, token)

		case at.TypeURC:
			// Handle URCs - could notify listeners or log
			// For now, ignore URCs during command execution
			continue

		case at.TypePrompt:
			// SMS prompt - return immediately for SMS handling
			lines = append(lines, token)
			return strings.Join(lines, "\n"), nil
		}
	}
}
