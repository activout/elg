package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/activout/elg/internal/config"
	"github.com/activout/elg/internal/secrets"
)

func TestRemainingCommandsUseRealSDKEndpoints(t *testing.T) {
	cases := []struct {
		args []string
		path string
	}{
		{[]string{"email", "list", "example.com"}, "/email/list"},
		{[]string{"email", "account", "list", "example.com"}, "/email/list"},
		{[]string{"email", "alias", "list", "example.com"}, "/email/list"},
		{[]string{"email", "alias", "get", "info@example.com"}, "/email/list"},
		{[]string{"email", "alias", "create", "info@example.com", "--to", "a@example.com"}, "/email/createalias"},
		{[]string{"email", "alias", "update", "info@example.com", "--to", "a@example.com"}, "/email/editalias"},
		{[]string{"email", "account", "delete", "a@example.com", "--yes"}, "/email/delete"},
		{[]string{"email", "costs"}, "/email/costs"},
		{[]string{"dns", "domain", "list"}, "/domain/list"},
		{[]string{"dns", "domain", "get", "example.com"}, "/domain/details"},
		{[]string{"dns", "domain", "export", "example.com"}, "/domain/export"},
		{[]string{"dns", "domain", "add", "example.com"}, "/domain/add"},
		{[]string{"dns", "domain", "delete", "example.com", "--yes"}, "/domain/delete"},
		{[]string{"dns", "record", "list", "example.com"}, "/domain/listrecords"},
		{[]string{"dns", "record", "delete", "12", "--yes"}, "/domain/deleterecord"},
		{[]string{"server", "get", "s1"}, "/server/details/serverid/s1/includestate/yes"},
		{[]string{"server", "templates"}, "/server/templates"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args[:min(3, len(tc.args))], " "), func(t *testing.T) {
			f := newFixture(t, nil)
			f.success(f.run(tc.args...))
			if got := f.exchanges()[len(f.exchanges())-1].path; got != tc.path {
				t.Fatalf("path %s want %s", got, tc.path)
			}
		})
	}
}
func TestCostNumbersPreserved(t *testing.T) {
	f := newFixture(t, nil)
	_, out, err := f.run("email", "costs")
	if err != "" || !strings.Contains(out, `"amount":0.125`) {
		t.Fatalf("cost type changed: %s %s", out, err)
	}
}
func TestSecretDeliveryFailureAfterSuccessfulMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if endpoint(r.URL.Path) == "email/list" {
			fmt.Fprint(w, `{"response":{"list":{"emailaccounts":[{"emailaccount":"a@example.com"}]}}}`)
			return
		}
		fmt.Fprint(w, `{"response":{}}`)
	})
	exit, out, err := f.run("email", "account", "reset-password", "a@example.com", "--yes", "--secret-output-file", path)
	errorCode(t, exit, out, err, 1, "secret_delivery_failed")
	if !strings.Contains(err, "mutation succeeded") {
		t.Fatal("failure hides known mutation success")
	}
	// A real write failure is converted to the same non-retryable delivery error.
	d, e := secrets.Reserve(filepath.Join(t.TempDir(), "closed"))
	if e != nil {
		t.Fatal(e)
	}
	d.Close()
	e = deliver(d, "returned password")
	if e == nil || !strings.Contains(e.Error(), "mutation succeeded") {
		t.Fatal("write failure did not preserve known state")
	}
}
func TestMutationTransportFailureIsUnknownAndNotRetried(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		h := w.(http.Hijacker)
		conn, _, err := h.Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
	exit, out, err := f.run("server", "start", "s1")
	errorCode(t, exit, out, err, 6, "mutation_outcome_unknown")
	if len(f.exchanges()) != 1 {
		t.Fatal("ambiguous mutation retried")
	}
}
func TestConfigurationLifecycleAndDryRun(t *testing.T) {
	var out, stderr strings.Builder
	path := filepath.Join(t.TempDir(), "config.json")
	run := func(args ...string) (int, string, string) {
		out.Reset()
		stderr.Reset()
		a := newApp(strings.NewReader(""), &out, &stderr, BuildInfo{Version: "test"})
		a.getenv = func(string) string { return "" }
		exit := a.execute(context.Background(), append(args, "--config", path, "--json"))
		return exit, out.String(), stderr.String()
	}
	check := func(exit int, out, err string) map[string]any { return success(t, exit, out, err) }
	check(run("config", "profile", "set", "personal", "--project", "CL123", "--api-key-env", "MY_KEY", "--dry-run"))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote config")
	}
	check(run("config", "profile", "set", "personal", "--project", "CL123", "--api-key-env", "MY_KEY"))
	check(run("config", "profile", "use", "personal"))
	c, err := config.Load(path)
	if err != nil || c.DefaultProfile != "personal" {
		t.Fatal("default profile not saved")
	}
	v := check(run("config", "profile", "list"))
	if v["data"].([]any)[0].(map[string]any)["api_key_env"] != "MY_KEY" {
		t.Fatal("environment reference incorrectly redacted")
	}
	check(run("config", "profile", "remove", "personal"))
	c, _ = config.Load(path)
	if len(c.Profiles) != 0 || c.DefaultProfile != "" {
		t.Fatal("profile removal incomplete")
	}
}
func TestUTF8AndPasswordNewlinePolicy(t *testing.T) {
	for _, tc := range []struct{ in, out string }{{" keep spaces \n", " keep spaces "}, {"keep\r\n", "keep"}, {"keep\r", "keep\r"}, {"keep\n\n", "keep\n"}} {
		p := filepath.Join(t.TempDir(), "password")
		_ = os.WriteFile(p, []byte(tc.in), 0600)
		got, err := passwordFile(p)
		if err != nil || got != tc.out {
			t.Fatalf("password bytes changed %q -> %q", tc.in, got)
		}
	}
	p := filepath.Join(t.TempDir(), "invalid-utf8")
	_ = os.WriteFile(p, []byte{0xff}, 0600)
	if _, err := textFile(p); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
func TestDryRunRedactsEvenWithShowSecrets(t *testing.T) {
	f := newFixture(t, nil)
	path := filepath.Join(t.TempDir(), "root-password")
	_ = os.WriteFile(path, []byte("provision-secret\n"), 0600)
	exit, out, err := f.run("server", "create", "--hostname", "test", "--datacenter", "Falkenberg", "--platform", "VMware", "--template", "t1", "--cpu-cores", "2", "--memory-mib", "2048", "--disk-gib", "20", "--root-password-file", path, "--show-secrets", "--dry-run")
	success(t, exit, out, err)
	if strings.Contains(out, "provision-secret") || len(f.exchanges()) > 0 {
		t.Fatal("dry-run leaked secret or mutated")
	}
	var v map[string]any
	_ = json.Unmarshal([]byte(out), &v)
	if v["data"].(map[string]any)["provider_validated"] != false {
		t.Fatal("dry-run claims provider validation")
	}
}
