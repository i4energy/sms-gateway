package modem

import (
	"time"
)

type Config struct {
	// dialer is the interface used to establish connection to the modem
	dialer Dialer
	// simPIN is the PIN code for SIM card authentication (optional)
	simPIN string
	// minSendInterval is the minimum time to wait between sending SMS messages
	minSendInterval time.Duration
	// maxRetries is the maximum number of retry attempts for failed operations
	maxRetries int
	// atTimeout is the timeout duration for individual AT command responses
	atTimeout time.Duration
	// initTimeout is the timeout duration for modem initialization sequence
	initTimeout time.Duration
}

// ConfigBuilder provides a fluent API for building modem configurations
type ConfigBuilder struct {
	config Config
}

// NewConfigBuilder creates a new ConfigBuilder with default values
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: Config{
			minSendInterval: time.Minute / 30,
			maxRetries:      3,
			atTimeout:       5 * time.Second,
			initTimeout:     30 * time.Second,
		},
	}
}

// WithDialer sets the dialer (required)
func (b *ConfigBuilder) WithDialer(dialer Dialer) *ConfigBuilder {
	b.config.dialer = dialer
	return b
}

// WithSimPIN sets the SIM PIN for authentication
func (b *ConfigBuilder) WithSimPIN(pin string) *ConfigBuilder {
	b.config.simPIN = pin
	return b
}

// WithMinSendInterval sets the minimum interval between SMS sends
func (b *ConfigBuilder) WithMinSendInterval(interval time.Duration) *ConfigBuilder {
	b.config.minSendInterval = interval
	return b
}

// WithMaxRetries sets the maximum number of retry attempts
func (b *ConfigBuilder) WithMaxRetries(retries int) *ConfigBuilder {
	b.config.maxRetries = retries
	return b
}

// WithATTimeout sets the timeout for AT commands
func (b *ConfigBuilder) WithATTimeout(timeout time.Duration) *ConfigBuilder {
	b.config.atTimeout = timeout
	return b
}

// WithInitTimeout sets the timeout for modem initialization
func (b *ConfigBuilder) WithInitTimeout(timeout time.Duration) *ConfigBuilder {
	b.config.initTimeout = timeout
	return b
}

// Build validates and returns the final configuration
func (b *ConfigBuilder) Build() (Config, error) {
	// Validate the configuration
	if b.config.dialer == nil {
		return b.config, ErrNoDialer
	}

	return b.config, nil
}
