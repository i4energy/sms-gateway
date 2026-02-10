package modem_test

import (
	"testing"

	"i4.energy/across/smsgw/modem"
)

func TestConfig(t *testing.T) {
	t.Run("ErrNoDialer when no dialer provided", func(t *testing.T) {
		_, err := modem.NewConfigBuilder().Build()

		if err != modem.ErrNoDialer {
			t.Errorf("expected ErrNoDialer, got: %v", err)
		}
	})
}
