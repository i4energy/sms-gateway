package at

import (
	"bytes"
	"strings"
)

// Splitter handles the "No Echo" scenario.
// It splits by CRLF but also recognizes the SMS prompt.
//
// The atEOF parameter indicates whether any more data will be available.
// When true, any remaining data is returned as the final token.
func Splitter(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// 1. Match SMS Prompt
	if bytes.HasPrefix(data, []byte(Prompt)) {
		return len(Prompt), data[0:len(Prompt)], nil
	}

	// 2. Match standard line ending with CRLF
	if i := bytes.Index(data, []byte(CRLF)); i >= 0 {
		return i + len(CRLF), data[0:i], nil
	}

	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// Classify identifies the nature of the modem output
func Classify(line string) ResponseType {
	if line == Prompt {
		return TypePrompt
	}

	// Direct matches for final results
	switch line {
	case OK, ERROR:
		return TypeFinal
	}

	// Prefix matches
	switch {
	case strings.HasPrefix(line, CmeError), strings.HasPrefix(line, CmsError):
		return TypeFinal
	case strings.HasPrefix(line, UrcNewMsg), line == UrcCall:
		return TypeURC
	default:
		return TypeData
	}
}
