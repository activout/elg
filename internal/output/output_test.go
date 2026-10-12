package output

import (
	"bytes"
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
