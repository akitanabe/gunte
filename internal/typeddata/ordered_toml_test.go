package typeddata

import (
	"testing"

	"github.com/BurntSushi/toml"
)

func TestOrderedTOMLValuePreservesNestedDeclarationOrderAndRejectsUnsupportedScalars(t *testing.T) {
	raw := map[string]any{"policy": map[string]any{"first": int64(1), "second": int64(2)}}
	value, ok := OrderedTOMLValue(raw, nil, []toml.Key{{"policy", "second"}, {"policy", "first"}})
	if !ok || len(value.Map) != 1 || value.Map[0].Key != "policy" || value.Map[0].Value.Map[0].Key != "second" {
		t.Fatalf("ordered value = %#v ok=%v", value, ok)
	}
	if _, ok := OrderedTOMLValue(map[string]any{"time": struct{}{}}, nil, []toml.Key{{"time"}}); ok {
		t.Fatal("unsupported scalar was accepted")
	}
}
