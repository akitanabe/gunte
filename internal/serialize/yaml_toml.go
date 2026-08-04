package serialize

import (
	"fmt"
	"strings"

	"github.com/akitanabe/gunte/internal/adapter"
	"github.com/akitanabe/gunte/internal/config"
)

func renderYAMLMetadata(fields []adapter.MetadataField) ([]byte, error) {
	var output strings.Builder
	seen := map[string]bool{}
	for _, field := range fields {
		if !validFieldName(field.Field) || yamlReservedWords[strings.ToLower(field.Field)] || seen[field.Field] {
			return nil, fmt.Errorf("invalid or duplicate YAML metadata field %q", field.Field)
		}
		seen[field.Field] = true
		value, err := renderYAMLValue(field.Value)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Field, err)
		}
		output.WriteString(field.Field)
		output.WriteString(": ")
		output.WriteString(value)
		output.WriteByte('\n')
	}
	return []byte(output.String()), nil
}

func renderYAMLValue(value adapter.MetadataValue) (string, error) {
	switch value.Type {
	case config.MetadataString:
		escaped, err := escapeString(value.String)
		if err != nil {
			return "", err
		}
		if value.String == "" {
			return "", fmt.Errorf("string value is empty")
		}
		return `"` + escaped + `"`, nil
	case config.MetadataStringList:
		values := make([]string, len(value.Strings))
		for i, item := range value.Strings {
			escaped, err := escapeString(item)
			if err != nil {
				return "", err
			}
			values[i] = `"` + escaped + `"`
		}
		return "[" + strings.Join(values, ", ") + "]", nil
	case config.MetadataCommaList:
		if len(value.Strings) == 0 {
			return "", fmt.Errorf("comma_list must not be empty")
		}
		values := make([]string, len(value.Strings))
		for i, item := range value.Strings {
			if !validYAMLToken(item) {
				return "", fmt.Errorf("invalid comma_list element %q", item)
			}
			values[i] = item
		}
		return strings.Join(values, ", "), nil
	case config.MetadataPlainToken:
		if !validYAMLToken(value.String) {
			return "", fmt.Errorf("invalid plain_token %q", value.String)
		}
		return value.String, nil
	default:
		return "", fmt.Errorf("metadata type %q is not supported by YAML", value.Type)
	}
}

func renderTOMLMetadata(fields []adapter.MetadataField) ([]byte, error) {
	var output strings.Builder
	seen := map[string]bool{}
	for _, field := range fields {
		if !validFieldName(field.Field) || seen[field.Field] {
			return nil, fmt.Errorf("invalid or duplicate TOML metadata field %q", field.Field)
		}
		seen[field.Field] = true
		var value string
		var err error
		switch field.Value.Type {
		case config.MetadataString:
			var escaped string
			escaped, err = escapeString(field.Value.String)
			value = `"` + escaped + `"`
		case config.MetadataStringList:
			items := make([]string, len(field.Value.Strings))
			for i, item := range field.Value.Strings {
				var escaped string
				escaped, err = escapeString(item)
				if err != nil {
					break
				}
				items[i] = `"` + escaped + `"`
			}
			value = "[" + strings.Join(items, ", ") + "]"
		default:
			return nil, fmt.Errorf("metadata type %q is not supported by TOML", field.Value.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Field, err)
		}
		output.WriteString(field.Field)
		output.WriteString(" = ")
		output.WriteString(value)
		output.WriteByte('\n')
	}
	return []byte(output.String()), nil
}

func validFieldName(value string) bool {
	if value == "" || !((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) {
		return false
	}
	for _, character := range value[1:] {
		if !((character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-') {
			return false
		}
	}
	return true
}

func validYAMLToken(value string) bool {
	return tokenPattern.MatchString(value) && !yamlReservedWords[strings.ToLower(value)]
}

func yamlFrontmatterEnd(body []byte) (int, bool) {
	if len(body) < 4 || !strings.HasPrefix(string(body), "---\n") {
		return 0, false
	}
	for start := 4; start <= len(body); {
		end := strings.IndexByte(string(body[start:]), '\n')
		if end < 0 {
			return 0, false
		}
		end += start
		if string(body[start:end]) == "---" {
			return end + 1, true
		}
		start = end + 1
	}
	return 0, false
}
