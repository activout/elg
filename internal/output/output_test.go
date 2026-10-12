package output

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestNestedSecretsAndAuthenticationNeverDisplayed(t *testing.T) {
	data := map[string]any{"password": "generated", "api_key": "auth", "credentials": map[string]any{"private_key": "private"}, "future": []any{map[string]any{"accessToken": "token", "authcode": "transfer", "other": "auth"}}}
	for _, show := range []bool{false, true} {
		var b bytes.Buffer
		if err := Write(&b, data, map[string]any{}, "json", false, show, "auth"); err != nil {
			t.Fatal(err)
		}
		s := b.String()
		if strings.Contains(s, `"auth"`) {
			t.Fatal("authentication key printed")
		}
		if !show && (strings.Contains(s, "generated") || strings.Contains(s, `"private"`) || strings.Contains(s, `"token"`) || strings.Contains(s, `"transfer"`)) {
			t.Fatal("nested secret printed")
		}
	}
}

func TestNDJSONPreservesPrecisionAndBuffersBeforeWriting(t *testing.T) {
	var b bytes.Buffer
	data := json.RawMessage(`[{"id":9007199254740993123,"unknown":{"decimal":0.1234567890123456789}},{"id":2}]`)
	if err := Write(&b, data, map[string]any{"command": "example list"}, "ndjson", false, false, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Count(b.String(), "\n") != 2 || !strings.Contains(b.String(), "9007199254740993123") || !strings.Contains(b.String(), "0.1234567890123456789") {
		t.Fatalf("NDJSON lost precision: %s", b.String())
	}
	b.Reset()
	if err := Write(&b, []any{1, math.Inf(1)}, nil, "ndjson", false, false, ""); err == nil || b.Len() != 0 {
		t.Fatal("encoding failure wrote partial NDJSON")
	}
}

func TestHumanTablesHaveSectionsAndUnquotedValues(t *testing.T) {
	var b bytes.Buffer
	data := map[string]any{"summary": map[string]any{"accounts": 1}, "domains": []any{map[string]any{"domainname": "example.com", "accounts": 1}}, "message": "line one\nline two\t\x1b"}
	if err := Write(&b, data, nil, "table", false, false, ""); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	if !strings.Contains(s, "summary:") || !strings.Contains(s, "domains:") || !strings.Contains(s, "example.com") || strings.Contains(s, `"example.com"`) || strings.Contains(s, "{") || strings.Contains(s, "\x1b") {
		t.Fatalf("unfriendly or unsafe table: %s", s)
	}
}

func TestHumanMixedListKeepsAllValues(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, []any{map[string]any{"name": "example.com"}, "standalone"}, nil, "table", false, false, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "example.com") || !strings.Contains(b.String(), "standalone") {
		t.Fatal("mixed list lost values")
	}
}
