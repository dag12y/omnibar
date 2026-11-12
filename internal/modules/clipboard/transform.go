package clipboard

import (
	"fmt"
	"strings"
)

// IsTransform lists the supported operations; arbitrary expressions are not run.
func IsTransform(operation string) bool {
	switch operation {
	case "trim", "lowercase", "uppercase", "remove_line_breaks":
		return true
	}
	return false
}
func transformText(text, operation string) (string, error) {
	switch operation {
	case "trim":
		text = strings.TrimSpace(text)
	case "lowercase":
		text = strings.ToLower(text)
	case "uppercase":
		text = strings.ToUpper(text)
	case "remove_line_breaks":
		// Treat CRLF as one break; use spaces to avoid joining separate words.
		text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\u2028", " ", "\u2029", " ").Replace(text)
	default:
		return "", fmt.Errorf("unsupported clipboard transform")
	}
	if text == "" {
		return "", fmt.Errorf("transform produced empty text; clipboard was unchanged")
	}
	if !validText(text) {
		return "", fmt.Errorf("transformed text exceeds the 16 KiB item limit or is invalid")
	}
	return text, nil
}
