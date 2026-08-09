package typeddata

import (
	"sort"

	"github.com/BurntSushi/toml"
)

// OrderedTOMLValue transforms decoded TOML Data while retaining declaration order.
func OrderedTOMLValue(raw any, prefix []string, keys []toml.Key) (Value, bool) {
	switch value := raw.(type) {
	case map[string]any:
		order, seen := []string{}, map[string]bool{}
		for _, key := range keys {
			if len(key) <= len(prefix) || !hasPrefix(key, prefix) || seen[key[len(prefix)]] {
				continue
			}
			seen[key[len(prefix)]] = true
			order = append(order, key[len(prefix)])
		}
		remaining := make([]string, 0)
		for key := range value {
			if !seen[key] {
				remaining = append(remaining, key)
			}
		}
		sort.Strings(remaining)
		order = append(order, remaining...)
		result := Value{Kind: Map}
		for _, key := range order {
			child, ok := OrderedTOMLValue(value[key], appendPath(prefix, key), keys)
			if !ok {
				return Value{}, false
			}
			result.Map = append(result.Map, Entry{Key: key, Value: child})
		}
		return result, true
	case []map[string]any:
		result := Value{Kind: List}
		for _, item := range value {
			child, ok := OrderedTOMLValue(item, prefix, keys)
			if !ok {
				return Value{}, false
			}
			result.List = append(result.List, child)
		}
		return result, true
	case []any:
		result := Value{Kind: List}
		for _, item := range value {
			child, ok := OrderedTOMLValue(item, prefix, keys)
			if !ok {
				return Value{}, false
			}
			result.List = append(result.List, child)
		}
		return result, true
	default:
		return FromAny(value)
	}
}

func hasPrefix(path toml.Key, prefix []string) bool {
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

func appendPath(prefix []string, key string) []string {
	result := append([]string(nil), prefix...)
	return append(result, key)
}
