package at_test

import (
	"bufio"
	"strings"
	"testing"

	"i4.energy/across/sms_gw/at"
)

func TestSplitter(t *testing.T) {
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
		// EOF scenarios - testing atEOF functionality
		{
			name:     "Incomplete command at EOF",
			input:    "AT+CSQ\r\n+CSQ: 15,99",
			expected: []string{"AT+CSQ", "+CSQ: 15,99"},
		},
		{
			name:     "Command without CRLF at EOF",
			input:    "AT+CPIN",
			expected: []string{"AT+CPIN"},
		},
		{
			name:     "SMS text without terminator at EOF",
			input:    "AT+CMGS=\"+123\"\r\n> Hello World",
			expected: []string{"AT+CMGS=\"+123\"", "> ", "Hello World"},
		},
		{
			name:     "Response cut off mid-stream at EOF",
			input:    "AT+CSQ\r\n+CSQ: 15,99\r\nOK\r\n+CMTI: \"SM\",1",
			expected: []string{"AT+CSQ", "+CSQ: 15,99", "OK", "+CMTI: \"SM\",1"},
		},
		{
			name:     "Partial SMS prompt at EOF",
			input:    "AT+CMGS=\"+123\"\r\n>",
			expected: []string{"AT+CMGS=\"+123\"", ">"},
		},
		{
			name:     "Mixed complete and incomplete at EOF",
			input:    "ATI\r\nQuectel\r\nBG96",
			expected: []string{"ATI", "Quectel", "BG96"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokens []string
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			scanner.Split(at.Splitter)

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
		expected at.ResponseType
	}{
		// Final responses
		{name: "OK response", input: "OK", expected: at.TypeFinal},
		{name: "ERROR response", input: "ERROR", expected: at.TypeFinal},
		{name: "CME Error", input: "+CME ERROR: 30", expected: at.TypeFinal},
		{name: "CMS Error", input: "+CMS ERROR: 500", expected: at.TypeFinal},
		// {name: "NO CARRIER", input: "NO CARRIER", expected: at.TypeFinal},

		// URCs
		{name: "New message URC", input: "+CMTI: \"SM\",1", expected: at.TypeURC},
		{name: "Incoming call URC", input: "RING", expected: at.TypeURC},

		// Data responses
		{name: "AT command", input: "AT+CSQ", expected: at.TypeData},
		{name: "Signal quality response", input: "+CSQ: 15,99", expected: at.TypeData},
		{name: "PIN status", input: "+CPIN: READY", expected: at.TypeData},
		{name: "Network registration", input: "+CREG: 0,1", expected: at.TypeData},
		{name: "SMS send result", input: "+CMGS: 123", expected: at.TypeData},
		{name: "Device info", input: "Quectel", expected: at.TypeData},

		// Prompt
		{name: "SMS input prompt", input: "> ", expected: at.TypePrompt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := at.Classify(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for input %q", tt.expected, result, tt.input)
			}
		})
	}
}
