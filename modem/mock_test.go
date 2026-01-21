package modem_test

import (
	gomock "go.uber.org/mock/gomock"
	"i4.energy/across/smsgw/modem"
)

type MockSequenceBuilder struct {
	transport *modem.MockTransport
	calls     []any
}

func NewMockSequence(transport *modem.MockTransport) *MockSequenceBuilder {
	return &MockSequenceBuilder{
		transport: transport,
		calls:     []any{},
	}
}

func (b *MockSequenceBuilder) AT() *MockSequenceBuilder {
	b.calls = append(b.calls,
		b.transport.EXPECT().Write([]byte("AT\r")).Return(3, nil),
		b.transport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
			resp := "AT\r\nOK\r\n"
			copy(p, resp)
			return len(resp), nil
		}),
	)
	return b
}

func (b *MockSequenceBuilder) EchoOff() *MockSequenceBuilder {
	b.calls = append(b.calls,
		b.transport.EXPECT().Write([]byte("ATE0\r")).Return(6, nil),
		b.transport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
			resp := "ATE0\r\nOK\r\n"
			copy(p, resp)
			return len(resp), nil
		}),
	)
	return b
}

func (b *MockSequenceBuilder) VerboseErrors() *MockSequenceBuilder {
	b.calls = append(b.calls,
		b.transport.EXPECT().Write([]byte("AT+CMEE=2\r")).Return(10, nil),
		b.transport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
			resp := "OK\r\n"
			copy(p, resp)
			return len(resp), nil
		}),
	)
	return b
}

func (b *MockSequenceBuilder) SimPinRequired() *MockSequenceBuilder {
	b.calls = append(b.calls,
		b.transport.EXPECT().Write([]byte("AT+CPIN?\r")).Return(9, nil),
		b.transport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
			resp := "+CPIN: SIM PIN\r\nOK\r\n"
			copy(p, resp)
			return len(resp), nil
		}),
	)
	return b
}

func (b *MockSequenceBuilder) SimReady() *MockSequenceBuilder {
	b.calls = append(b.calls,
		b.transport.EXPECT().Write([]byte("AT+CPIN?\r")).Return(9, nil),
		b.transport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
			resp := "+CPIN: READY\r\nOK\r\n"
			copy(p, resp)
			return len(resp), nil
		}),
	)
	return b
}

func (b *MockSequenceBuilder) SMSTextMode() *MockSequenceBuilder {
	b.calls = append(b.calls,
		b.transport.EXPECT().Write([]byte("AT+CMGF=1\r")).Return(10, nil),
		b.transport.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
			resp := "OK\r\n"
			copy(p, resp)
			return len(resp), nil
		}),
	)
	return b
}

func (b *MockSequenceBuilder) Build() []any {
	return b.calls
}
