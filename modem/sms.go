package modem

import (
	"context"
	"fmt"
	"strings"

	"i4.energy/across/smsgw/at"
)

// SMS represents a text message stored on the modem.
type SMS struct {
	Index  int
	Status string // "REC UNREAD", "REC READ", "STO UNSENT", "STO SENT"
	Sender string
	Time   string
	Text   string
}

// SendSMS sends a text message to the specified recipient.
//
// The message is sent in text mode (not PDU mode). The recipient should be
// in international format (e.g., "+1234567890").
//
// This method blocks until the message is accepted by the network or an error
// occurs. Network delivery (to the final recipient) happens asynchronously.
func (m *Modem) SendSMS(ctx context.Context, recipient, message string) error {
	// Use exec to send the initial command and get the prompt
	resp, err := m.exec(ctx, fmt.Sprintf(`AT+CMGS="%s"`, recipient))
	if err != nil {
		return fmt.Errorf("AT+CMGS command failed: %w", err)
	}

	// Check if we got the prompt
	if !strings.Contains(resp, at.Prompt) {
		return fmt.Errorf("did not receive SMS prompt, got: %q", resp)
	}

	// Now send the message body and wait for confirmation
	// This is essentially another exec(), but we just send the message text
	messageCmd := message + at.CtrlZ
	resp, err = m.exec(ctx, messageCmd)
	if err != nil {
		return fmt.Errorf("SMS send failed: %w", err)
	}

	// Check for successful send (should contain +CMGS and OK)
	if !strings.Contains(resp, at.OK) {
		return fmt.Errorf("unexpected SMS response: %s", resp)
	}

	return nil
}
