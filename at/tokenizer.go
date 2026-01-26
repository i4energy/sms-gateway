package at

import (
	"bufio"
	"bytes"
	"strings"
)

// Splitter is used for tokenizing AT command modem responses. It uses
// the signature of bufio.SplitFunc so it can be directly used with bufio.Scanner.
//
// It splits the input by CRLF line endings and also
// recognizes the SMS input prompt ("> ").
//
// Important: This splitter assumes "No Echo" mode (ATE0). If echo is enabled,
// it would need modification to handle command echoes that precede the actual
// response.
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

var _ bufio.SplitFunc = Splitter

// Classify identifies the nature of the modem output
func Classify(line string) ResponseType {
	if line == Prompt {
		return TypePrompt
	}

	// Direct matches for final results
	switch line {
	case OK, ERROR, NoCarrier, NoDialtone, Busy, NoAnswer:
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
