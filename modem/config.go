package modem

import (
	"time"
)

func (c *Config) validate() error {
	if c.Dialer == nil {
		return ErrNoDialer
	}
	return nil
}

type Config struct {
	Dialer          Dialer
	SimPIN          string
	MinSendInterval time.Duration
	MaxRetries      int
	EchoOn          bool
	ATTimeout       time.Duration
	InitTimeout     time.Duration
}

func (c *Config) setDefaults() {
	if c.MinSendInterval == 0 {
		c.MinSendInterval = time.Minute / 30
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 3
	}
	if c.ATTimeout == 0 {
		c.ATTimeout = 5 * time.Second
	}
	if c.InitTimeout == 0 {
		c.InitTimeout = 30 * time.Second
	}
}
