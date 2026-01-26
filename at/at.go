// Package at provides parsing and tokenization utilities for AT command protocol
// communication with GSM modems.
//
// AT commands are the standard interface for controlling GSM/cellular modems,
// originally developed for Hayes-compatible modems. This package handles the
// text-based request-response protocol, including proper line termination,
// response classification, and special cases like SMS text entry prompts.
//
// # Protocol Overview
//
// AT commands follow a structured pattern:
//  1. Commands are sent with CRLF termination
//  2. Responses arrive as CRLF-terminated lines
//  3. Commands conclude with final result codes (OK, ERROR, etc.)
//  4. Intermediate data may be returned before the final result
//  5. Unsolicited Result Codes (URCs) can arrive asynchronously
//
// # No Echo Mode
//
// This package assumes "No Echo" mode (ATE0) where commands are not echoed
// back by the modem. The Splitter function is specifically designed for this
// mode and would require modification for echo mode operation.
//
// # Usage Example
//
//	// Tokenize modem responses
//	scanner := bufio.NewScanner(modemReader)
//	scanner.Split(at.Splitter)
//
//	for scanner.Scan() {
//		line := scanner.Text()
//		responseType := at.Classify(line)
//
//		switch responseType {
//		case at.TypeFinal:
//			// Command completed
//		case at.TypeData:
//			// Process intermediate data
//		case at.TypeURC:
//			// Handle asynchronous notification
//		case at.TypePrompt:
//			// SMS text entry mode
//		}
//	}
//
// # Key Components
//
//   - Constants: Standard AT command strings and response codes
//   - Splitter: bufio.SplitFunc for tokenizing modem output
//   - Classify: Response type classification for proper handling
//   - ResponseType: Enum for different kinds of modem responses
package at

const (
	// Terminal Control
	CRLF   = "\r\n"
	Prompt = "> "
	CtrlZ  = "\x1A"

	// Response Codes
	OK         = "OK"
	ERROR      = "ERROR"
	NoCarrier  = "NO CARRIER"
	NoDialtone = "NO DIALTONE"
	Busy       = "BUSY"
	NoAnswer   = "NO ANSWER"
	CmeError   = "+CME ERROR:"
	CmsError   = "+CMS ERROR:"
	SimReady   = "+CPIN: READY"
	SimPin     = "+CPIN: SIM PIN"

	// Commands
	CmdAt            = "AT"
	CmdEchoOff       = "ATE0"
	CmdSetTextMode   = "AT+CMGF=1"
	CmdVerboseErrors = "AT+CMEE=2"
	CmdSimStatus     = "AT+CPIN?"

	// URCs (Unsolicited Result Codes)
	UrcNewMsg         = "+CMTI:"
	UrcMessageReport  = "+CDSI:"
	UrcSignalStrength = "+CSQ:"
	UrcCall           = "RING"
)

// ResponseType classifies the nature of AT command modem responses for parsing
// and flow control purposes.
//
// AT command communication follows a structured protocol where different response
// types require different handling strategies. This classification enables the
// command processor to determine appropriate next actions, such as continuing
// to read more data, processing intermediate results, or concluding command
// execution.
type ResponseType int

const (
	// TypeFinal indicates command completion responses that terminate AT command
	// execution. These responses signal that no additional output should be
	// expected for the current command.
	//
	// Examples: "OK", "ERROR", "+CME ERROR: 30", "NO CARRIER"
	TypeFinal ResponseType = iota

	// TypeURC represents Unsolicited Result Codes - asynchronous notifications
	// from the modem that are not direct responses to AT commands. These can
	// arrive at any time and should be processed separately from command flows.
	//
	// Examples: "+CMTI: \"SM\",1" (new SMS), "RING" (incoming call)
	TypeURC

	// TypeData represents intermediate command output that provides requested
	// information but does not indicate command completion. Commands may return
	// multiple TypeData responses followed by a TypeFinal response.
	//
	// Examples: "+CSQ: 15,99" (signal quality), "+CPIN: READY" (SIM status)
	TypeData

	// TypePrompt indicates the SMS text input prompt ("> ") which signals
	// that the modem is ready to accept SMS message content. This requires
	// special handling as it's neither command output nor a final response.
	//
	// Example: "> " (SMS composition prompt)
	TypePrompt
)
