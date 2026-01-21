package at

const (
	// Terminal Control
	CRLF   = "\r\n"
	Prompt = "> "

	// Response Codes
	OK         = "OK"
	ERROR      = "ERROR"
	NoCarrier  = "NO CARRIER"
	NoDialtone = "NO DIALTONE"
	Busy       = "BUSY"
	NoAnswer   = "NO ANSWER"
	CmeError   = "+CME ERROR:"
	CmsError   = "+CMS ERROR:"

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
