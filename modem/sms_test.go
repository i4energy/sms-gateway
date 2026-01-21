package modem_test

import (
	context "context"
	"errors"
	"io"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"
	"i4.energy/across/smsgw/modem"
)

// TestModem_SendSMS_Success verifies that SendSMS correctly implements the
// AT command protocol sequence for sending SMS messages:
//
//  1. Write: AT+CMGS="+1234567890"\r
//  2. Read:  "> " (wait for prompt)
//  3. Write: "Hello World\x1a\r" (only after receiving prompt)
//  4. Read:  "+CMGS: 123\r\nOK\r\n" (wait for confirmation)
//
// This sequence must be strictly ordered - writing the message body before
// receiving the prompt will fail with real modem hardware.
//
// # Test Coordination
//
// Since reads and writes happen across different goroutines in the implementation,
// this test uses coordination channels to ensure deterministic execution:
//
//  1. allowRead: Blocks the response Read until after the message body Write.
//     This ensures the test enforces the correct protocol ordering - responses
//     must not be available until after their corresponding writes complete.
//
//  2. allowEOF: Blocks the EOF Read until after SendSMS completes.
//     This prevents goroutines from terminating before SendSMS finishes
//     processing all responses.
//
// Without this coordination, the test would be flaky due to non-deterministic
// goroutine scheduling - the reader goroutine could issue reads at unpredictable
// times, potentially receiving EOF before SendSMS finishes processing.
func TestModem_SendSMS_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransport := modem.NewMockTransport(ctrl)
	mockDialer := modem.NewMockDialer(ctrl)

	gomock.InOrder(
		slices.Concat(
			[]any{
				mockDialer.EXPECT().Dial(gomock.Any()).Return(mockTransport, nil),
			},
			initMockCalls(mockTransport),
		)...,
	)

	ctx := context.Background()
	m, err := modem.New(ctx, modem.Config{Dialer: mockDialer})
	if err != nil {
		t.Fatalf("failed to create modem: %v", err)
	}
	defer m.Close()

	go func() {
		if err := m.Loop(ctx); err != nil && err != context.Canceled && err != io.EOF {
			t.Errorf("modem loop error: %v", err)
		}
	}()

	// Channels to coordinate Read/Write ordering between goroutines.
	// Reader goroutines can issue reads at any time (non-deterministic scheduling).
	// These channels ensure reads happen in the correct sequence relative to writes,
	// simulating the natural blocking behavior of real hardware.
	allowRead := make(chan struct{})
	allowEOF := make(chan struct{})

	mockTransport.EXPECT().Write([]byte(`AT+CMGS="+1234567890"` + "\r"))
	mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
		return copy(p, "> "), nil
	})
	mockTransport.EXPECT().Write([]byte("Hello World\x1a\r")).Do(func([]byte) {
		close(allowRead) // Allow second Read after second Write
	})
	mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
		<-allowRead // Block until message body is written
		return copy(p, "+CMGS: 123\r\nOK\r\n"), nil
	})
	// Block until we signal it's safe to return EOF
	mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
		<-allowEOF
		return 0, io.EOF
	})
	mockTransport.EXPECT().Close().Return(nil)

	err = m.SendSMS(ctx, "+1234567890", "Hello World")
	close(allowEOF) // SendSMS completed, allow EOF now
	if err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("unexpected error: %v", err)
	}
}
