package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestEveryCommandGroupDefaultsToHumanOutputWhenRedirected(t *testing.T) {
	for _, args := range [][]string{
		{"version"}, {"config", "show"}, {"config", "profile", "list"},
		{"email", "overview"}, {"email", "list", "example.com"}, {"email", "account", "list", "example.com"}, {"email", "costs"},
		{"dns", "domain", "list"}, {"dns", "domain", "export", "example.com"},
		{"server", "list"}, {"server", "templates"}, {"server", "stop", "s1", "--dry-run"},
		{"api", "call", "email/costs"}, {"email", "alias", "create", "info@example.com", "--to", "a@example.com"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := newFixture(t, nil)
			exit, out, err := f.runWith(context.Background(), nil, "", args...)
			if exit != 0 || err != "" || out == "" || json.Valid([]byte(out)) {
				t.Fatalf("human output: exit=%d out=%s err=%s", exit, out, err)
			}
			if strings.Contains(out, `{"`) || strings.Contains(out, `"emailaccount"`) || strings.Contains(out, "must-hide") {
				t.Fatalf("JSON fragment or secret in human output: %s", out)
			}
		})
	}
}
func TestOnlyExplicitMachineFlagsEnableMachineOutput(t *testing.T) {
	f := newFixture(t, nil)
	env := map[string]string{"ELG_OUTPUT": "json", "ELG_PROJECT": "CLfixture", "ELG_API_KEY": "fixture-secret"}
	exit, out, err := f.runWith(context.Background(), env, "", "version")
	if exit != 0 || err != "" || json.Valid([]byte(out)) {
		t.Fatal("environment enabled JSON")
	}
	for _, args := range [][]string{{"version", "--json=false"}, {"version", "--ndjson=false"}} {
		exit, out, err = f.runWith(context.Background(), env, "", args...)
		if exit != 0 || err != "" || json.Valid([]byte(out)) {
			t.Fatal("false flag enabled JSON")
		}
	}
	for _, flag := range []string{"--json", "--ndjson"} {
		exit, out, err = f.runWith(context.Background(), nil, "", "version", flag)
		success(t, exit, out, err)
	}
	exit, out, err = f.runWith(context.Background(), nil, "", "version", "--pretty")
	if exit != 2 || out != "" || !strings.HasPrefix(err, "Error:") {
		t.Fatal("pretty enabled JSON or invalid human error")
	}
	exit, out, err = f.runWith(context.Background(), nil, "", "version", "--json", "--ndjson")
	errorCode(t, exit, out, err, 2, "invalid_input")
	exit, out, err = f.runWith(context.Background(), nil, "", "version", "--ndjson", "--pretty")
	errorCode(t, exit, out, err, 2, "invalid_input")
	exit, out, err = f.runWith(context.Background(), nil, "", "version", "--output", "json")
	if exit != 2 || out != "" || json.Valid([]byte(err)) {
		t.Fatal("legacy output flag still enables JSON")
	}
}
func TestNDJSONListRowsAndRedaction(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"response":{"list":{"emailaccounts":[{"emailaccount":"a@example.com","password":"secret-one"},{"emailaccount":"b@example.com","password":"secret-two"}]}}}`)
	})
	exit, out, err := f.runWith(context.Background(), nil, "", "email", "account", "list", "example.com", "--ndjson")
	if exit != 0 || err != "" {
		t.Fatalf("exit=%d err=%s", exit, err)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two NDJSON records: %s", out)
	}
	for i, line := range lines {
		v := success(t, 0, line, "")
		data := v["data"].(map[string]any)
		if data["emailaccount"] != fmt.Sprintf("%c@example.com", 'a'+i) || data["password"] != "[REDACTED]" || v["meta"].(map[string]any)["command"] != "email account list" {
			t.Fatalf("unexpected NDJSON row: %s", line)
		}
	}
	if strings.Contains(out, "secret-one") || strings.Contains(out, "secret-two") {
		t.Fatal("NDJSON secret exposed")
	}
	empty := newFixture(t, nil)
	exit, out, err = empty.runWith(context.Background(), nil, "", "server", "list", "--ndjson")
	if exit != 0 || out != "" || err != "" {
		t.Fatal("empty list produced records")
	}
	exit, out, err = empty.runWith(context.Background(), nil, "", "server", "start", "s1", "--ndjson")
	v := success(t, exit, out, err)
	if v["data"] != nil || v["meta"].(map[string]any)["operation"] != "start" {
		t.Fatal("no-data operation lost metadata")
	}
}
func TestDefaultAndNDJSONErrorsIncludingParserFailures(t *testing.T) {
	f := newFixture(t, nil)
	for _, args := range [][]string{{"email", "account", "get"}, {"--not-a-flag"}, {"server", "stop", "s1"}} {
		exit, out, err := f.runWith(context.Background(), nil, "", args...)
		if exit == 0 || out != "" || !strings.HasPrefix(err, "Error:") || json.Valid([]byte(err)) {
			t.Fatalf("human error: exit=%d out=%s err=%s", exit, out, err)
		}
		exit, out, err = f.runWith(context.Background(), nil, "", append(args, "--ndjson")...)
		wantExit, wantCode := 2, "invalid_input"
		if args[0] == "server" {
			wantExit, wantCode = 8, "confirmation_required"
		}
		errorCode(t, exit, out, err, wantExit, wantCode)
	}
}
