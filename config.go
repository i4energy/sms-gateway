package main

import (
	"flag"
	"os"
	"strconv"
)

// Config holds the application configuration
type Config struct {
	// BindAddress is the address the server listens on (e.g. "0.0.0.0:8080")
	BindAddress string
	// SerialPort is the path to the modem's serial port (e.g. "/dev/ttyUSB0")
	SerialPort string
	// BaudRate is the baud rate for serial communication with the modem (e.g. 115200)
	BaudRate int
	// LogLevel sets the logging level (e.g. "debug", "info", "warn", "error")
	LogLevel string
	// SimPIN is the SIM card PIN code
	SimPIN string
}

// ConfigOption is a function that modifies a Config
type ConfigOption func(*Config) error

// LoadConfig creates a new config by applying the given options in order
func LoadConfig(opts ...ConfigOption) (*Config, error) {
	config := &Config{}

	for _, opt := range opts {
		if err := opt(config); err != nil {
			return nil, err
		}
	}

	return config, nil
}

// WithDefaults applies default configuration values
func WithDefaults() ConfigOption {
	return func(c *Config) error {
		c.BindAddress = "0.0.0.0:8080"
		c.SerialPort = "/dev/ttyUSB0"
		c.BaudRate = 115200
		c.LogLevel = "info"
		return nil
	}
}

// WithEnv loads configuration from environment variables
func WithEnv() ConfigOption {
	return func(c *Config) error {
		if addr := os.Getenv("BIND_ADDRESS"); addr != "" {
			c.BindAddress = addr
		}

		if serial := os.Getenv("SERIAL_PORT"); serial != "" {
			c.SerialPort = serial
		}

		if baud := os.Getenv("BAUD_RATE"); baud != "" {
			if b, err := strconv.Atoi(baud); err == nil {
				c.BaudRate = b
			}
		}

		if level := os.Getenv("LOG_LEVEL"); level != "" {
			c.LogLevel = level
		}

		if simPIN := os.Getenv("SIM_PIN"); simPIN != "" {
			c.SimPIN = simPIN
		}

		return nil
	}
}

// WithFlags loads configuration from command-line flags
func WithFlags(fSet *flag.FlagSet) ConfigOption {
	return func(c *Config) error {
		fSet.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "bind-address":
				c.BindAddress = f.Value.String()
			case "serial-port":
				c.SerialPort = f.Value.String()
			case "baud-rate":
				if b, err := strconv.Atoi(f.Value.String()); err == nil {
					c.BaudRate = b
				}
			case "log-level":
				c.LogLevel = f.Value.String()
			case "sim-pin":
				c.SimPIN = f.Value.String()
			}

		})
		return nil
	}

}
