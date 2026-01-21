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
)

type Modem struct {
	mu        sync.Mutex
	transport Transport
	config    Config
	reader    *bufio.Reader
}

func New(ctx context.Context, config Config) (*Modem, error) {
	config.setDefaults()
	if err := config.validate(); err != nil {
		return nil, err
	}

	transport, err := config.Dialer.Dial()
	if err != nil {
		return nil, err
	}

	m := &Modem{
		config:    config,
		transport: transport,
		reader:    bufio.NewReaderSize(transport, 16*1024),
	}

	// Initialize the modem (e.g., send AT commands to set it up)
	transport.Write([]byte("AT\r\n"))

	return m, nil
}

func (m *Modem) readLine() (string, error) {
	line, err := m.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
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

		line, err := m.readLine()
		if err != nil {
			return strings.Join(lines, "\n"), err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Ignore echoed command if echo is enabled
		if m.config.EchoOn && line == strings.TrimSpace(cmd) {
			continue
		}

		lines = append(lines, line)

		switch {
		case line == "OK":
			return strings.Join(lines, "\n"), nil

		case line == "ERROR":
			return strings.Join(lines, "\n"), errors.New("modem returned ERROR")

		case strings.HasPrefix(line, "+CME ERROR"):
			return strings.Join(lines, "\n"), errors.New(line)
		}
	}
}
