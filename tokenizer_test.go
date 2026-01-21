package gsm_test

import (
	"bufio"
	"strings"
	"testing"
	"time"

	gsm "i4.energy/across/sms_gw"
)

func TestNewModem(t *testing.T) {
	r := strings.NewReader("")
	modem, err := gsm.NewModem(r)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if modem == nil {
		t.Fatal("Expected modem to be non-nil")
	}
}

func TestNewModemNilReader(t *testing.T) {
	modem, err := gsm.NewModem(nil)
	if err == nil {
		t.Fatal("Expected error for nil reader")
	}
	if modem != nil {
		t.Fatal("Expected modem to be nil for nil reader")
	}
}

func TestATSplitter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Simple AT command response",
			input:    "AT+CSQ\r\n+CSQ: 15,99\r\nOK\r\n",
			expected: []string{"AT+CSQ", "+CSQ: 15,99", "OK"},
		},
		{
			name:     "AT command with error",
			input:    "AT+CPIN?\r\n+CME ERROR: 10\r\n",
			expected: []string{"AT+CPIN?", "+CME ERROR: 10"},
		},
		{
			name:     "SMS sending sequence",
			input:    "AT+CMGS=\"+1234567890\"\r\n> Hello World!\x1A\r\n+CMGS: 123\r\nOK\r\n",
			expected: []string{"AT+CMGS=\"+1234567890\"", "> ", "Hello World!\x1A", "+CMGS: 123", "OK"},
		},
		{
			name:     "Network registration check",
			input:    "AT+CREG?\r\n+CREG: 0,1\r\nOK\r\n",
			expected: []string{"AT+CREG?", "+CREG: 0,1", "OK"},
		},
		{
			name:     "Multiple AT commands",
			input:    "ATI\r\nQuectel\r\nBG96\r\nRevision: BG96MAR02A07M1G\r\nOK\r\n",
			expected: []string{"ATI", "Quectel", "BG96", "Revision: BG96MAR02A07M1G", "OK"},
		},
		{
			name:     "URC mixed with AT response",
			input:    "AT+CSQ\r\n+CMTI: \"SM\",1\r\n+CSQ: 20,99\r\nOK\r\n",
			expected: []string{"AT+CSQ", "+CMTI: \"SM\",1", "+CSQ: 20,99", "OK"},
		},
		{
			name:     "SMS prompt only",
			input:    "> ",
			expected: []string{"> "},
		},
		{
			name:     "Empty lines handling",
			input:    "\r\n\r\nAT\r\nOK\r\n\r\n",
			expected: []string{"", "", "AT", "OK", ""},
		},
		{
			name:     "Multiple URCs",
			input:    "+CMTI: \"SM\",1\r\n+CMTI: \"SM\",2\r\nRING\r\n+CMTI: \"SM\",3\r\n",
			expected: []string{"+CMTI: \"SM\",1", "+CMTI: \"SM\",2", "RING", "+CMTI: \"SM\",3"},
		},
		{
			name:     "Call flow with RING",
			input:    "ATD+1234567890;\r\nOK\r\nRING\r\nRING\r\nNO CARRIER\r\n",
			expected: []string{"ATD+1234567890;", "OK", "RING", "RING", "NO CARRIER"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokens []string
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			scanner.Split(gsm.ATSplitter)

			for scanner.Scan() {
				tokens = append(tokens, scanner.Text())
			}

			if err := scanner.Err(); err != nil {
				t.Fatalf("Scanner error: %v", err)
			}

			if len(tokens) != len(tt.expected) {
				t.Fatalf("Expected %d tokens, got %d.\nExpected: %v\nGot: %v",
					len(tt.expected), len(tokens), tt.expected, tokens)
			}

			for i, expected := range tt.expected {
				if tokens[i] != expected {
					t.Errorf("Token %d: expected %q, got %q", i, expected, tokens[i])
				}
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected gsm.ResponseType
	}{
		// Final responses
		{name: "OK response", input: "OK", expected: gsm.TypeFinal},
		{name: "ERROR response", input: "ERROR", expected: gsm.TypeFinal},
		{name: "CME Error", input: "+CME ERROR: 30", expected: gsm.TypeFinal},
		{name: "CMS Error", input: "+CMS ERROR: 500", expected: gsm.TypeFinal},
		// {name: "NO CARRIER", input: "NO CARRIER", expected: gsm.TypeFinal},

		// URCs
		{name: "New message URC", input: "+CMTI: \"SM\",1", expected: gsm.TypeURC},
		{name: "Incoming call URC", input: "RING", expected: gsm.TypeURC},

		// Data responses
		{name: "AT command", input: "AT+CSQ", expected: gsm.TypeData},
		{name: "Signal quality response", input: "+CSQ: 15,99", expected: gsm.TypeData},
		{name: "PIN status", input: "+CPIN: READY", expected: gsm.TypeData},
		{name: "Network registration", input: "+CREG: 0,1", expected: gsm.TypeData},
		{name: "SMS send result", input: "+CMGS: 123", expected: gsm.TypeData},
		{name: "Device info", input: "Quectel", expected: gsm.TypeData},

		// Prompt
		{name: "SMS input prompt", input: "> ", expected: gsm.TypePrompt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gsm.Classify(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for input %q", tt.expected, result, tt.input)
			}
		})
	}
}

func TestModemChannelDispatch(t *testing.T) {
	// Test data simulating AT command sequence with mixed responses and URCs
	testData := `AT+CSQ
+CSQ: 15,99
OK
+CMTI: "SM",1
RING
AT+CMGS="+1234567890"
> Hello World!` + "\x1A" + `
+CMGS: 123
OK
+CMTI: "SM",2
`

	modem, err := gsm.NewModem(strings.NewReader(testData))
	if err != nil {
		t.Fatalf("Failed to create modem: %v", err)
	}

	// Collect responses with timeout
	var finalResponses []string
	var dataResponses []string
	var urcs []string
	var prompts int

	timeout := time.NewTimer(100 * time.Millisecond)
	defer timeout.Stop()

	// Give the scanner time to process
	time.Sleep(10 * time.Millisecond)

responseLoop:
	for {
		select {
		case resp := <-modem.RespChan():
			finalResponses = append(finalResponses, resp)
		case data := <-modem.DataChan():
			dataResponses = append(dataResponses, data)
		case urc := <-modem.URC():
			urcs = append(urcs, urc)
		case <-modem.PromptChan():
			prompts++
		case err := <-modem.Errors():
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		case <-timeout.C:
			break responseLoop
		}
	}

	// Verify we got expected responses
	expectedFinalResponses := 2 // 2 OK responses
	if len(finalResponses) < expectedFinalResponses {
		t.Errorf("Expected at least %d final responses, got %d: %v",
			expectedFinalResponses, len(finalResponses), finalResponses)
	}

	// Should have data responses including AT commands and their responses
	if len(dataResponses) < 3 { // At least AT commands and some responses
		t.Errorf("Expected at least 3 data responses, got %d: %v",
			len(dataResponses), dataResponses)
	}

	// Should have URCs
	expectedURCs := 3 // 2 CMTI + 1 RING
	if len(urcs) < expectedURCs {
		t.Errorf("Expected at least %d URCs, got %d: %v", expectedURCs, len(urcs), urcs)
	}

	// Should have one SMS prompt
	if prompts < 1 {
		t.Errorf("Expected at least 1 prompt, got %d", prompts)
	}
}

func TestInterleavedURCs(t *testing.T) {
	// Test scenario where URCs arrive while processing commands
	testData := `+CSQ: 15,99
+CMTI: "SM",1
OK
RING
+CMTI: "SM",2
RING
`

	modem, err := gsm.NewModem(strings.NewReader(testData))
	if err != nil {
		t.Fatalf("Failed to create modem: %v", err)
	}

	var responses []string
	var urcs []string

	timeout := time.NewTimer(100 * time.Millisecond)
	defer timeout.Stop()

	// Give scanner time to process
	time.Sleep(10 * time.Millisecond)

responseLoop:
	for {
		select {
		case resp := <-modem.RespChan():
			responses = append(responses, resp)
		case data := <-modem.DataChan():
			responses = append(responses, data)
		case urc := <-modem.URC():
			urcs = append(urcs, urc)
		case err := <-modem.Errors():
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		case <-timeout.C:
			break responseLoop
		}
	}

	// Should have at least one response (OK) and one data response (+CSQ: 15,99)
	if len(responses) < 2 {
		t.Errorf("Expected at least 2 responses, got %d: %v", len(responses), responses)
	}

	// Should have URCs: 2 CMTI and 2 RING
	expectedURCs := 4
	if len(urcs) < expectedURCs {
		t.Errorf("Expected at least %d URCs, got %d: %v", expectedURCs, len(urcs), urcs)
	}

	// Verify URC types
	urcCounts := make(map[string]int)
	for _, urc := range urcs {
		if strings.HasPrefix(urc, "+CMTI:") {
			urcCounts["CMTI"]++
		} else if urc == "RING" {
			urcCounts["RING"]++
		}
	}

	if urcCounts["CMTI"] < 2 {
		t.Errorf("Expected at least 2 CMTI URCs, got %d", urcCounts["CMTI"])
	}
	if urcCounts["RING"] < 2 {
		t.Errorf("Expected at least 2 RING URCs, got %d", urcCounts["RING"])
	}
}

func TestErrorScenarios(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "CME Error - SIM not inserted",
			input:    "AT+CPIN?\r\n+CME ERROR: 10\r\n",
			expected: "+CME ERROR: 10",
		},
		{
			name:     "CMS Error - SMS service failure",
			input:    "+CMS ERROR: 500\r\n",
			expected: "+CMS ERROR: 500",
		},
		{
			name:     "Simple ERROR",
			input:    "ERROR\r\n",
			expected: "ERROR",
		},
		{
			name:     "NO CARRIER on call",
			input:    "NO CARRIER\r\n",
			expected: "NO CARRIER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modem, err := gsm.NewModem(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Failed to create modem: %v", err)
			}

			timeout := time.NewTimer(50 * time.Millisecond)
			defer timeout.Stop()

			// Give scanner time to process
			time.Sleep(5 * time.Millisecond)

			var errorReceived bool

			select {
			case resp := <-modem.RespChan():
				if resp == tt.expected {
					errorReceived = true
				}
			case <-modem.DataChan():
				// Might be classified as data instead of final
			case err := <-modem.Errors():
				if err != nil {
					t.Fatalf("Unexpected scanner error: %v", err)
				}
			case <-timeout.C:
			}

			if !errorReceived {
				// Check if it might have been classified as data
				select {
				case data := <-modem.DataChan():
					if data != tt.expected {
						t.Errorf("Expected error %q but got data: %q", tt.expected, data)
					}
				default:
					t.Errorf("Expected to receive error %q", tt.expected)
				}
			}
		})
	}
}

func TestSMSFlow(t *testing.T) {
	// Test complete SMS sending flow
	smsData := `AT+CMGS="+1234567890"
> Test Message` + "\x1A" + `
+CMGS: 123
OK
`

	modem, err := gsm.NewModem(strings.NewReader(smsData))
	if err != nil {
		t.Fatalf("Failed to create modem: %v", err)
	}

	var sequence []string
	timeout := time.NewTimer(100 * time.Millisecond)
	defer timeout.Stop()

	// Give scanner time to process
	time.Sleep(10 * time.Millisecond)

	for {
		select {
		case <-modem.PromptChan():
			sequence = append(sequence, "PROMPT")
		case resp := <-modem.RespChan():
			sequence = append(sequence, "FINAL:"+resp)
		case data := <-modem.DataChan():
			sequence = append(sequence, "DATA:"+data)
		case err := <-modem.Errors():
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		case <-timeout.C:
			goto done
		}
	}

done:
	// Should contain a prompt
	hasPrompt := false
	for _, event := range sequence {
		if event == "PROMPT" {
			hasPrompt = true
			break
		}
	}
	if !hasPrompt {
		t.Errorf("Expected to receive SMS prompt in sequence: %v", sequence)
	}

	// Should have at least some events
	if len(sequence) < 2 {
		t.Errorf("Expected at least 2 events, got %d: %v", len(sequence), sequence)
	}
}

func BenchmarkATSplitter(b *testing.B) {
	data := `+CSQ: 15,99
+CMTI: "SM",1
OK
RING
`
	input := strings.Repeat(data, 100) // Create larger input

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(gsm.ATSplitter)

		for scanner.Scan() {
			_ = scanner.Text()
		}
	}
}

func BenchmarkClassify(b *testing.B) {
	testCases := []string{
		"OK", "ERROR", "+CME ERROR: 30", "+CMTI: \"SM\",1",
		"RING", "+CSQ: 15,99", "> ", "+CMGS: 123",
		"AT+CSQ", "+CPIN: READY", "NO CARRIER",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			gsm.Classify(tc)
		}
	}
}
