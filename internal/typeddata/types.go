// Package typeddata provides ordered, strictly typed tree Data for structural checks.
package typeddata

type Kind string

const (
	String Kind = "string"
	Int    Kind = "int64"
	Bool   Kind = "bool"
	List   Kind = "list"
	Map    Kind = "map"
)

type Entry struct {
	Key   string
	Value Value
}

type Value struct {
	Kind   Kind
	String string
	Int    int64
	Bool   bool
	List   []Value
	Map    []Entry
}

func FromAny(input any) (Value, bool) {
	switch value := input.(type) {
	case string:
		return Value{Kind: String, String: value}, true
	case int64:
		return Value{Kind: Int, Int: value}, true
	case bool:
		return Value{Kind: Bool, Bool: value}, true
	case []any:
		result := Value{Kind: List, List: make([]Value, 0, len(value))}
		for _, item := range value {
			child, ok := FromAny(item)
			if !ok {
				return Value{}, false
			}
			result.List = append(result.List, child)
		}
		return result, true
	case map[string]any:
		result := Value{Kind: Map}
		for key, raw := range value {
			child, ok := FromAny(raw)
			if !ok {
				return Value{}, false
			}
			result.Map = append(result.Map, Entry{Key: key, Value: child})
		}
		return result, true
	default:
		return Value{}, false
	}
}

func Equal(a, b Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case String:
		return a.String == b.String
	case Int:
		return a.Int == b.Int
	case Bool:
		return a.Bool == b.Bool
	case List:
		if len(a.List) != len(b.List) {
			return false
		}
		for i := range a.List {
			if !Equal(a.List[i], b.List[i]) {
				return false
			}
		}
		return true
	case Map:
		if len(a.Map) != len(b.Map) {
			return false
		}
		for _, ae := range a.Map {
			found := false
			for _, be := range b.Map {
				if ae.Key == be.Key {
					found = Equal(ae.Value, be.Value)
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	default:
		return false
	}
}
