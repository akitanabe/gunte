// Package canonicaljson provides compact JSON encoding for string Data.
package canonicaljson

import (
	"fmt"
	"strings"
)

func StringArray(values []string) string {
	encoded := make([]string, len(values))
	for index, value := range values {
		encoded[index] = String(value)
	}
	return "[" + strings.Join(encoded, ",") + "]"
}

func String(value string) string {
	var result strings.Builder
	result.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			result.WriteByte('\\')
			result.WriteRune(r)
		case '\b':
			result.WriteString("\\b")
		case '\t':
			result.WriteString("\\t")
		case '\n':
			result.WriteString("\\n")
		case '\f':
			result.WriteString("\\f")
		case '\r':
			result.WriteString("\\r")
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&result, "\\u%04x", r)
			} else {
				result.WriteRune(r)
			}
		}
	}
	result.WriteByte('"')
	return result.String()
}
