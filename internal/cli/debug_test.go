package cli

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestDebugJSONFailureHasOneRedactedErrorOnly(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"response":{"status":{"code":403,"text":"fixture-secret refused"}}}`)
	})
	exit, out, err := f.run("server", "list", "--debug", "--json")
	errorCode(t, exit, out, err, 3, "authentication_error")
	if strings.Contains(err, "fixture-secret") {
		t.Fatal("debug/error exposed authentication")
	}
}
