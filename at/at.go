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

type ResponseType int

const (
	TypeFinal  ResponseType = iota // OK, ERROR
	TypeURC                        // Asynchronous notifications
	TypeData                       // Intermediate command output (+CSQ: ...)
	TypePrompt                     // SMS input prompt
)
