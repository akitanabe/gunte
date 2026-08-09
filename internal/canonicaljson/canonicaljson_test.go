package canonicaljson

import "testing"

func TestStringAndStringArrayUseCompactNormativeEscaping(t *testing.T) {
	if got := String("<>&é\u2028\u007f\n\""); got != `"<>&é \u007f\n\""` {
		t.Fatalf("string = %q", got)
	}
	if got := StringArray([]string{"a", "é"}); got != `["a","é"]` {
		t.Fatalf("array = %q", got)
	}
}
