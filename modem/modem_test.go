package modem_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"
	"i4.energy/across/smsgw/modem"
)

func TestNew_NoDialer(t *testing.T) {
	config := modem.Config{
		Dialer: nil,
	}

	ctx := context.Background()
	m, err := modem.New(ctx, config)

	if err != modem.ErrNoDialer {
		t.Errorf("expected ErrNoDialer, got: %v", err)
	}
	if m != nil {
		t.Error("New() should return nil modem when no dialer")
	}
}

func TestNew_DialerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDialer := modem.NewMockDialer(ctrl)
	mockDialer.EXPECT().Dial(gomock.Any()).Return(nil, errors.New("connection failed"))

	config := modem.Config{
		Dialer: mockDialer,
	}

	ctx := context.Background()
	m, err := modem.New(ctx, config)

	if err == nil {
		t.Error("expected error from dialer failure")
	}
	if m != nil {
		t.Error("New() should return nil modem when dialer fails")
	}
}

// initMockCalls returns a slice of expected calls for successful modem initialization.
// It can be used in all tests that require a successfully initialized modem.
func initMockCalls(mockTransport *modem.MockTransport) []any {
	return NewMockSequence(mockTransport).
		AT().
		EchoOff().
		VerboseErrors().
		SimReady().
		SMSTextMode().
		Build()
}

func TestNew_InitializationSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransport := modem.NewMockTransport(ctrl)
	mockDialer := modem.NewMockDialer(ctrl)

	gomock.InOrder(slices.Concat(
		[]any{
			mockDialer.EXPECT().Dial(gomock.Any()).Return(mockTransport, nil),
		},
		initMockCalls(mockTransport),
	)...)

	m, err := modem.New(context.Background(), modem.Config{
		Dialer: mockDialer,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if m == nil {
		t.Error("New() should return valid modem on success")
	}

	// Clean up
	mockTransport.EXPECT().Close().Return(nil)
	_ = m.Close()
}

func TestNew_SIMPINRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransport := modem.NewMockTransport(ctrl)
	mockDialer := modem.NewMockDialer(ctrl)

	calls := NewMockSequence(mockTransport).
		AT().
		EchoOff().
		VerboseErrors().
		SimPinRequired().
		Build()

	gomock.InOrder(
		slices.Concat(
			[]any{
				mockDialer.EXPECT().Dial(gomock.Any()).Return(mockTransport, nil),
			},
			calls,
			[]any{
				mockTransport.EXPECT().Close(),
			},
		)...,
	)

	m, err := modem.New(context.Background(), modem.Config{
		Dialer: mockDialer,
	})

	if !errors.Is(err, modem.ErrSIMPinRequired) {
		t.Errorf("expected ErrSIMPinRequired, got: %v", err)
	}
	if m != nil {
		t.Error("New() should return nil modem when SIM PIN required")
	}
}

// func TestModem_Close(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()

// 	mockTransport := NewMockTransport(ctrl)
// 	mockTransport.EXPECT().Close().Return(nil)

// 	modem := &Modem{
// 		transport: mockTransport,
// 		config:    Config{},
// 		closed:    false,
// 	}

// 	err := modem.Close()
// 	if err != nil {
// 		t.Errorf("unexpected error: %v", err)
// 	}

// 	// Second close should return ErrAlreadyClosed
// 	err = modem.Close()
// 	if err != ErrAlreadyClosed {
// 		t.Errorf("expected ErrAlreadyClosed, got: %v", err)
// 	}
// }

// func TestModem_Close_TransportError(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()

// 	mockTransport := NewMockTransport(ctrl)
// 	closeError := errors.New("transport close failed")
// 	mockTransport.EXPECT().Close().Return(closeError)

// 	modem := &Modem{
// 		transport: mockTransport,
// 		config:    Config{},
// 		closed:    false,
// 	}

// 	err := modem.Close()
// 	if err != closeError {
// 		t.Errorf("expected transport error, got: %v", err)
// 	}
// }

// func TestModem_exec_AlreadyClosed(t *testing.T) {
// 	modem := &Modem{
// 		closed: true,
// 	}

// 	ctx := context.Background()
// 	resp, err := modem.exec(ctx, "AT")

// 	if err != ErrAlreadyClosed {
// 		t.Errorf("expected ErrAlreadyClosed, got: %v", err)
// 	}
// 	if resp != "" {
// 		t.Errorf("expected empty response, got: %q", resp)
// 	}
// }

// func TestModem_exec_NotInitialized(t *testing.T) {
// 	modem := &Modem{
// 		transport: nil,
// 		closed:    false,
// 	}

// 	ctx := context.Background()
// 	resp, err := modem.exec(ctx, "AT")

// 	if err != ErrNotInitialized {
// 		t.Errorf("expected ErrNotInitialized, got: %v", err)
// 	}
// 	if resp != "" {
// 		t.Errorf("expected empty response, got: %q", resp)
// 	}
// }

// func TestModem_exec_Success(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()

// 	mockTransport := NewMockTransport(ctrl)
// 	mockTransport.EXPECT().Write([]byte("AT\r")).Return(3, nil)
// 	mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
// 		resp := "AT\r\nOK\r\n"
// 		copy(p, resp)
// 		return len(resp), nil
// 	})

// 	modem := &Modem{
// 		transport: mockTransport,
// 		config:    Config{ATTimeout: 5 * time.Second},
// 		closed:    false,
// 	}

// 	ctx := context.Background()
// 	resp, err := modem.exec(ctx, "AT")

// 	if err != nil {
// 		t.Errorf("unexpected error: %v", err)
// 	}
// 	if !strings.Contains(resp, "OK") {
// 		t.Errorf("expected OK in response, got: %q", resp)
// 	}
// }

// func TestModem_exec_Error(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()

// 	mockTransport := NewMockTransport(ctrl)
// 	mockTransport.EXPECT().Write([]byte("ATXXX\r")).Return(6, nil)
// 	mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
// 		resp := "ATXXX\r\nERROR\r\n"
// 		copy(p, resp)
// 		return len(resp), nil
// 	})

// 	modem := &Modem{
// 		transport: mockTransport,
// 		config:    Config{ATTimeout: 5 * time.Second},
// 		closed:    false,
// 	}

// 	ctx := context.Background()
// 	resp, err := modem.exec(ctx, "ATXXX")

// 	if err == nil {
// 		t.Errorf("expected error got response %v", resp)
// 	}
// 	if err.Error() != "ERROR" {
// 		t.Errorf("expected 'ERROR', got: %v", err)
// 	}
// }

// func TestModem_exec_WriteError(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()

// 	writeError := errors.New("write failed")
// 	mockTransport := NewMockTransport(ctrl)
// 	mockTransport.EXPECT().Write([]byte("AT\r")).Return(0, writeError)

// 	modem := &Modem{
// 		transport: mockTransport,
// 		config:    Config{ATTimeout: 5 * time.Second},
// 		closed:    false,
// 	}

// 	ctx := context.Background()
// 	resp, err := modem.exec(ctx, "AT")

// 	if err == nil {
// 		t.Errorf("expected error from write failure, got response %v", resp)
// 	}
// 	if !strings.Contains(err.Error(), "write command") {
// 		t.Errorf("expected write error, got: %v", err)
// 	}
// }

// func TestModem_exec_ContextCanceled(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()

// 	mockTransport := NewMockTransport(ctrl)
// 	mockTransport.EXPECT().Write([]byte("AT\r")).Return(3, nil)
// 	mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
// 		// Simulate slow response
// 		time.Sleep(100 * time.Millisecond)
// 		return 0, io.EOF
// 	}).AnyTimes()

// 	modem := &Modem{
// 		transport: mockTransport,
// 		config:    Config{},
// 		closed:    false,
// 	}

// 	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
// 	defer cancel()

// 	resp, err := modem.exec(ctx, "AT")

// 	if err == nil {
// 		t.Errorf("expected context timeout error, got response %v", resp)
// 	}
// 	if !errors.Is(err, context.DeadlineExceeded) && err != io.EOF {
// 		t.Errorf("expected context error or EOF, got: %v", err)
// 	}
// }

// func TestConfig_setDefaults(t *testing.T) {
// 	config := Config{}
// 	config.setDefaults()

// 	if config.MinSendInterval != time.Minute/30 {
// 		t.Errorf("expected MinSendInterval %v, got %v", time.Minute/30, config.MinSendInterval)
// 	}
// 	if config.MaxRetries != 3 {
// 		t.Errorf("expected MaxRetries 3, got %d", config.MaxRetries)
// 	}
// 	if config.ATTimeout != 5*time.Second {
// 		t.Errorf("expected ATTimeout %v, got %v", 5*time.Second, config.ATTimeout)
// 	}
// 	if config.InitTimeout != 30*time.Second {
// 		t.Errorf("expected InitTimeout %v, got %v", 30*time.Second, config.InitTimeout)
// 	}
// }

// func TestConfig_setDefaults_PreservesExisting(t *testing.T) {
// 	config := modem.Config{
// 		MinSendInterval: time.Second,
// 		MaxRetries:      5,
// 		ATTimeout:       10 * time.Second,
// 		InitTimeout:     60 * time.Second,
// 	}
// 	config.setDefaults()

// 	if config.MinSendInterval != time.Second {
// 		t.Errorf("expected preserved MinSendInterval %v, got %v", time.Second, config.MinSendInterval)
// 	}
// 	if config.MaxRetries != 5 {
// 		t.Errorf("expected preserved MaxRetries 5, got %d", config.MaxRetries)
// 	}
// 	if config.ATTimeout != 10*time.Second {
// 		t.Errorf("expected preserved ATTimeout %v, got %v", 10*time.Second, config.ATTimeout)
// 	}
// 	if config.InitTimeout != 60*time.Second {
// 		t.Errorf("expected preserved InitTimeout %v, got %v", 60*time.Second, config.InitTimeout)
// 	}
// }
