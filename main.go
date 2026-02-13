package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"i4.energy/across/smsgw/modem"
)

func main() {
	flag.String("serial-port", "/dev/ttyUSB0", "Serial port to connect to the modem")
	flag.Int("baud-rate", 115200, "Baud rate for serial communication")
	flag.String("bind-address", "0.0.0.0:8080", "Bind address for the HTTP server")
	flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.String("sim-pin", "", "SIM card PIN code (if required)")
	flag.Parse()

	config, err := LoadConfig(WithDefaults(), WithEnv(), WithFlags(flag.CommandLine))
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	logLevel := slog.LevelInfo
	switch config.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	modemConfig, err := modem.NewConfigBuilder().
		WithATTimeout(5 * time.Second).
		WithInitTimeout(30 * time.Second).
		WithMaxRetries(5).
		WithMinSendInterval(10 * time.Second).
		WithSimPIN(config.SimPIN).
		WithDialer(modem.SerialDialer{
			PortName: config.SerialPort,
			BaudRate: config.BaudRate,
		}).
		Build()
	if err != nil {
		logger.Error("Failed to create modem config", "error", err)
		os.Exit(1)
	}

	m, err := modem.New(context.Background(), modemConfig)
	if err != nil {
		logger.Error("Failed to create modem", "error", err)
		os.Exit(1)
	}

	logger.Info("Starting SMS Gateway", "modem", m)

	httpServer := &http.Server{
		Addr: config.BindAddress,
		Handler: &Server{
			Logger: logger.With("component", "server"),
			Modem:  m,
		},
	}

	// Channel to listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server in a goroutine
	go func() {
		logger.Info("Starting HTTP server", "address", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	sig := <-sigChan
	logger.Info("Received shutdown signal", "signal", sig)

	logger.Info("Closing modem connection")
	if err := m.Close(); err != nil {
		logger.Error("Failed to close modem", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Info("Closing HTTP server")
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("Failed to gracefully shutdown server", "error", err)
		os.Exit(1)
	}
}
