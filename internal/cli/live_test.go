package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var liveEnvFile = flag.String("live-env-file", "", "Opt-in read-only smoke test using credential names from an env file")

// This test only calls commands listed as verified reads; it never prints resources.
func TestLiveReadOnly(t *testing.T) {
	if *liveEnvFile == "" {
		t.Skip("use -args -live-env-file=/path/to/.env to opt in")
	}
	b, err := os.ReadFile(*liveEnvFile)
	if err != nil {
		t.Fatal("cannot read live environment file")
	}
	env := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || (key != "ELG_API_KEY" && key != "ELG_PROJECT" && key != "ELG_PROJECT_ID") {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, `"`) {
			v, e := strconv.Unquote(value)
			if e != nil {
				t.Fatal("invalid quoted credential in env file")
			}
			value = v
		} else if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
			value = value[1 : len(value)-1]
		}
		env[key] = value
	}
	if env["ELG_API_KEY"] == "" || (env["ELG_PROJECT"] == "" && env["ELG_PROJECT_ID"] == "") {
		t.Fatal("env file must provide ELG_API_KEY and ELG_PROJECT or ELG_PROJECT_ID")
	}
	run := func(args ...string) map[string]any {
		t.Helper()
		var out, stderr bytes.Buffer
		a := newApp(strings.NewReader(""), &out, &stderr, BuildInfo{"test", "unknown", "test"})
		a.getenv = func(k string) string { return env[k] }
		exit := a.execute(context.Background(), append(args, "--json", "--non-interactive", "--config", filepath.Join(t.TempDir(), "config.json")))
		if exit != 0 {
			var failure struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(stderr.Bytes(), &failure)
			t.Fatalf("read-only command %s failed: exit %d, code %s: %s", strings.Join(args[:min(2, len(args))], " "), exit, failure.Error.Code, failure.Error.Message)
		}
		var v map[string]any
		if err := json.Unmarshal(out.Bytes(), &v); err != nil {
			t.Fatal("invalid live JSON envelope")
		}
		return v
	}
	overview := run("email", "overview")
	t.Log("email overview: OK")
	domains := overview["data"].(map[string]any)["domains"].([]any)
	if len(domains) > 0 {
		domain := domains[0].(map[string]any)["domainname"].(string)
		all := run("email", "list", domain)
		run("email", "account", "list", domain)
		run("email", "alias", "list", domain)
		accounts := all["data"].(map[string]any)["emailaccounts"].([]any)
		if len(accounts) > 0 {
			address := accounts[0].(map[string]any)["emailaccount"].(string)
			run("email", "account", "get", address)
			run("email", "account", "quota", address)
		}
		aliases := all["data"].(map[string]any)["emailaliases"].([]any)
		if len(aliases) > 0 {
			run("email", "alias", "get", aliases[0].(map[string]any)["emailalias"].(string))
		}
		t.Log("email list, account/alias lists and available exact lookups/quota: OK")
	}
	run("email", "costs")
	t.Log("email costs: OK")
	dns := run("dns", "domain", "list")["data"].([]any)
	if len(dns) > 0 {
		domain := dns[0].(map[string]any)["domainname"].(string)
		run("dns", "domain", "get", domain)
		run("dns", "domain", "export", domain)
		run("dns", "record", "list", domain)
	}
	t.Log("DNS reads: OK")
	servers := run("server", "list")["data"].([]any)
	if len(servers) > 0 {
		run("server", "get", servers[0].(map[string]any)["serverid"].(string))
	}
	run("server", "templates")
	t.Log("server reads: OK")
	run("api", "call", "email/costs")
	t.Log("read-only generic POST: OK")
}
