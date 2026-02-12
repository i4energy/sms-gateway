package modem_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"i4.energy/across/smsgw/modem"
)

func TestModemNew(t *testing.T) {
	t.Run("Initialization Success", func(t *testing.T) {
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

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()

		if err != nil {
			t.Errorf("unexpected error from Build(): %v", err)
		}
		m, err := modem.New(context.Background(), config)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if m == nil {
			t.Error("New() should return valid modem on success")
		}

		// Clean up
		mockTransport.EXPECT().Close().Return(nil)
		if err := m.Close(); err != nil {
			t.Errorf("unexpected error from Close(): %v", err)
		}
	})

	t.Run("ErrSIMPinRequired when SIM PIN is required but not provided", func(t *testing.T) {
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

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()
		if err != nil {
			t.Errorf("unexpected error from Build(): %v", err)
		}

		m, err := modem.New(context.Background(), config)
		if !errors.Is(err, modem.ErrSIMPinRequired) {
			t.Errorf("expected ErrSIMPinRequired, got: %v", err)
		}
		if m != nil {
			t.Error("New() should return nil modem when error occurs")
		}
	})

	t.Run("Dialer error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockDialer := modem.NewMockDialer(ctrl)
		mockDialer.EXPECT().Dial(gomock.Any()).Return(nil, errors.New("connection failed"))

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()

		if err != nil {
			t.Errorf("unexpected error from Build(): %v", err)
		}

		ctx := context.Background()
		m, err := modem.New(ctx, config)

		if err == nil {
			t.Error("expected error from dialer failure")
		}
		if m != nil {
			t.Error("New() should return nil modem when dialer fails")
		}
	})

	t.Run("ErrNoDialer when no dialer provided", func(t *testing.T) {
		m, err := modem.New(context.Background(), modem.Config{})
		if !errors.Is(err, modem.ErrNoDialer) {
			t.Errorf("expected ErrNoDialer from New(), got: %v", err)
		}
		if m != nil {
			t.Error("New() should return nil modem when no dialer provided")
		}
	})

	t.Run("ErrNotInitialized on nil transport", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockDialer := modem.NewMockDialer(ctrl)
		mockDialer.EXPECT().Dial(gomock.Any()).Return(nil, nil)

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()

		if err != nil {
			t.Errorf("unexpected error from Build(): %v", err)
		}

		// This should create a modem with nil transport, putting it in "not initialized" state
		_, err = modem.New(context.Background(), config)
		if !errors.Is(err, modem.ErrNotInitialized) {
			t.Errorf("expected ErrNotInitialized from New(), got: %v", err)
		}

	})
}

func TestModemClose(t *testing.T) {
	t.Run("Closes underlying transport successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransport := modem.NewMockTransport(ctrl)
		mockDialer := modem.NewMockDialer(ctrl)

		gomock.InOrder(slices.Concat(
			[]any{
				mockDialer.EXPECT().Dial(gomock.Any()).Return(mockTransport, nil),
			},
			initMockCalls(mockTransport),
			[]any{
				mockTransport.EXPECT().Close().Return(nil),
			},
		)...)

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()

		if err != nil {
			t.Errorf("unexpected error from Build(): %v", err)
		}

		m, err := modem.New(context.Background(), config)
		if err != nil {
			t.Errorf("unexpected error from New(): %v", err)
		}

		if err := m.Close(); err != nil {
			t.Errorf("unexpected error from Close(): %v", err)
		}
	})

	t.Run("Returns transport error on close failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransport := modem.NewMockTransport(ctrl)
		mockDialer := modem.NewMockDialer(ctrl)

		closeError := errors.New("transport close failed")
		gomock.InOrder(slices.Concat(
			[]any{
				mockDialer.EXPECT().Dial(gomock.Any()).Return(mockTransport, nil),
			},
			initMockCalls(mockTransport),
			[]any{
				mockTransport.EXPECT().Close().Return(closeError),
			},
		)...)

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()

		if err != nil {
			t.Errorf("unexpected error from Build(): %v", err)
		}

		m, err := modem.New(context.Background(), config)
		if err != nil {
			t.Errorf("unexpected error from New(): %v", err)
		}

		if err := m.Close(); err != closeError {
			t.Errorf("expected transport error, got: %v", err)
		}
	})

	t.Run("ErrAlreadyClosed on double close", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransport := modem.NewMockTransport(ctrl)
		mockDialer := modem.NewMockDialer(ctrl)

		// Set up successful initialization
		gomock.InOrder(slices.Concat(
			[]any{
				mockDialer.EXPECT().Dial(gomock.Any()).Return(mockTransport, nil),
			},
			initMockCalls(mockTransport),
			[]any{
				mockTransport.EXPECT().Close().Return(nil),
			},
		)...)

		// Expect Close to be called once successfully

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()

		if err != nil {
			t.Errorf("unexpected error from Build(): %v", err)
		}

		m, err := modem.New(context.Background(), config)
		if err != nil {
			t.Errorf("unexpected error from New(): %v", err)
		}
		if m == nil {
			t.Error("New() should return valid modem on success")
		}

		// First close should succeed
		err = m.Close()
		if err != nil {
			t.Errorf("first close should succeed, got error: %v", err)
		}

		// Second close should return ErrAlreadyClosed
		err = m.Close()
		if err != modem.ErrAlreadyClosed {
			t.Errorf("expected ErrAlreadyClosed on second close, got: %v", err)
		}
	})
}

func TestModemLoop(t *testing.T) {
	t.Run("Starts and stops on EOF", func(t *testing.T) {
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

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()
		if err != nil {
			t.Fatalf("unexpected error from Build(): %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		m, err := modem.New(ctx, config)
		if err != nil {
			t.Fatalf("failed to create modem: %v", err)
		}
		defer m.Close()

		// This test verifies Loop handles normal transport I/O
		allowEOF := make(chan struct{})

		// Loop should read continuously until context cancellation or EOF
		mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
			<-allowEOF
			return 0, io.EOF
		})
		mockTransport.EXPECT().Close().Return(nil)

		// Start Loop in goroutine and verify it runs until EOF
		loopDone := make(chan error, 1)
		go func() {
			loopDone <- m.Loop(ctx)
		}()

		// Signal EOF and wait for Loop to complete
		close(allowEOF)
		err = <-loopDone

		if err != nil && !errors.Is(err, io.EOF) {
			t.Errorf("expected Loop to handle EOF gracefully, got: %v", err)
		}
	})

	t.Run("Dispatch URCs to the designated channel", func(t *testing.T) {
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

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()
		if err != nil {
			t.Fatalf("unexpected error from Build(): %v", err)
		}

		ctx := context.Background()
		m, err := modem.New(ctx, config)
		if err != nil {
			t.Fatalf("failed to create modem: %v", err)
		}
		defer m.Close()

		// Coordinate reads to ensure URC is processed before EOF
		allowEOF := make(chan struct{})

		// First read returns a URC, second returns EOF
		gomock.InOrder(
			mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
				return copy(p, "+CMTI: \"SM\",1\r\n"), nil // New SMS URC
			}),
			mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
				<-allowEOF
				return 0, io.EOF
			}),
		)
		mockTransport.EXPECT().Close().Return(nil)

		// Start Loop
		loopDone := make(chan error, 1)
		go func() {
			loopDone <- m.Loop(ctx)
		}()

		// Check that URC is received on URC channel
		select {
		case urc := <-m.URC():
			if !strings.Contains(urc, "+CMTI:") {
				t.Errorf("expected URC to contain +CMTI:, got: %q", urc)
			}
		case <-time.After(time.Second):
			t.Error("expected URC to be received within timeout")
		}

		// Signal EOF and wait for Loop to finish
		close(allowEOF)
		err = <-loopDone

		if err != nil && !errors.Is(err, io.EOF) {
			t.Errorf("expected Loop to handle EOF gracefully, got: %v", err)
		}
	})

	t.Run("Exits gracefully on context cancellation", func(t *testing.T) {
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

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()
		if err != nil {
			t.Fatalf("unexpected error from Build(): %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		m, err := modem.New(ctx, config)
		if err != nil {
			t.Fatalf("failed to create modem: %v", err)
		}
		defer m.Close()

		// Coordinate cancellation timing
		readStarted := make(chan struct{})

		// Read should block until context is cancelled
		mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
			close(readStarted)
			// Block until cancelled
			<-ctx.Done()
			return 0, ctx.Err()
		})
		mockTransport.EXPECT().Close().Return(nil)

		// Start Loop
		loopDone := make(chan error, 1)
		go func() {
			loopDone <- m.Loop(ctx)
		}()

		// Wait for read to start, then cancel
		<-readStarted
		cancel()

		// Verify Loop was cancelled properly
		err = <-loopDone
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected Loop to return context.Canceled, got: %v", err)
		}
	})

	t.Run("Handle scanner errors from Transport", func(t *testing.T) {
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

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()
		if err != nil {
			t.Fatalf("unexpected error from Build(): %v", err)
		}

		ctx := context.Background()
		m, err := modem.New(ctx, config)
		if err != nil {
			t.Fatalf("failed to create modem: %v", err)
		}
		defer m.Close()

		scannerError := errors.New("transport read error")

		// Read should return an error
		mockTransport.EXPECT().Read(gomock.Any()).Return(0, scannerError)
		mockTransport.EXPECT().Close().Return(nil)

		// Loop should propagate scanner errors
		err = m.Loop(ctx)
		if err == nil {
			t.Error("expected Loop to return scanner error")
		}
		if !strings.Contains(err.Error(), "scanner error") {
			t.Errorf("expected scanner error to be wrapped, got: %v", err)
		}
	})

	t.Run("ErrLoopRunning on consecutive calls", func(t *testing.T) {
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

		config, err := modem.NewConfigBuilder().
			WithDialer(mockDialer).
			Build()
		if err != nil {
			t.Fatalf("unexpected error from Build(): %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		m, err := modem.New(ctx, config)
		if err != nil {
			t.Fatalf("failed to create modem: %v", err)
		}
		defer m.Close()

		// Set up minimal expectations for first Loop
		mockTransport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
			<-ctx.Done()
			return 0, ctx.Err()
		}).AnyTimes()
		mockTransport.EXPECT().Close().Return(nil)

		// Start first Loop in background
		loopDone := make(chan error, 1)
		go func() {
			loopDone <- m.Loop(ctx)
		}()

		// Give first Loop time to start and set loopRunning flag
		time.Sleep(10 * time.Millisecond)

		// Try to start second Loop - should fail immediately
		err = m.Loop(ctx)
		if !errors.Is(err, modem.ErrLoopRunning) {
			t.Errorf("expected ErrLoopRunning, got: %v", err)
		}

		// Clean up first Loop
		cancel()
		<-loopDone
	})
}
