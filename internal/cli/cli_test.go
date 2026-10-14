package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/activout/elg/internal/config"
)

type exchange struct {
	path, method string
	body         map[string]any
}
type fixture struct {
	mu       sync.Mutex
	t        *testing.T
	server   *httptest.Server
	requests []exchange
	handler  func(http.ResponseWriter, *http.Request)
}

func newFixture(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *fixture {
	f := &fixture{t: t, handler: handler}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, k, ok := r.BasicAuth()
		if !ok || p == "" || k != "fixture-secret" {
			t.Error("unexpected authentication")
		}
		if !strings.Contains(r.UserAgent(), "elg/test glesys-go/") {
			t.Error("unexpected user agent")
		}
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		f.mu.Lock()
		f.requests = append(f.requests, exchange{r.URL.EscapedPath(), r.Method, body})
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if handler != nil {
			handler(w, r)
			return
		}
		switch endpoint(r.URL.Path) {
		case "email/list":
			fmt.Fprint(w, `{"response":{"list":{"emailaccounts":[{"emailaccount":"a@example.com","quotaingib":2,"password":"must-hide"}],"emailaliases":[{"emailalias":"info@example.com","goto":"a@example.com"}]}}}`)
		case "email/quota":
			fmt.Fprint(w, `{"response":{"quota":{"emailaccount":"a@example.com","usedquotainmib":1024,"quotaingib":2}}}`)
		case "email/resetpassword":
			fmt.Fprint(w, `{"response":{"password":"new secret with spaces "}}`)
		case "email/createaccount", "email/editaccount":
			fmt.Fprint(w, `{"response":{"emailaccount":{"emailaccount":"a@example.com","password":"new secret with spaces "}}}`)
		case "email/createalias", "email/editalias":
			fmt.Fprint(w, `{"response":{"alias":{"emailalias":"info@example.com","goto":"a@example.com"}}}`)
		case "email/overview":
			fmt.Fprint(w, `{"response":{"overview":{"summary":{"accounts":1},"domains":[],"meta":{"page":1,"total":0,"perpage":30}}}}`)
		case "email/costs":
			fmt.Fprint(w, `{"response":{"costs":{"quota":{"amount":2,"cost":{"amount":0.125,"currency":"SEK"}}},"pricelist":{"quota":{"amount":0.125,"currency":"SEK","unit":"GiB"}}}}`)
		case "domain/list":
			fmt.Fprint(w, `{"response":{"domains":[]}}`)
		case "domain/add", "domain/details":
			fmt.Fprint(w, `{"response":{"domain":{"domainname":"example.com"}}}`)
		case "domain/listrecords":
			fmt.Fprint(w, `{"response":{"records":[]}}`)
		case "domain/addrecord", "domain/updaterecord":
			fmt.Fprint(w, `{"response":{"record":{"recordid":12}}}`)
		case "domain/export":
			fmt.Fprint(w, `{"response":{"zonefile":"example.com. 60 IN A 192.0.2.1"}}`)
		case "server/list":
			fmt.Fprint(w, `{"response":{"servers":[]}}`)
		case "server/templates":
			fmt.Fprint(w, `{"response":{"templates":{"KVM":[],"VMware":[]}}}`)
		case "server/create", "server/edit", "server/details":
			fmt.Fprint(w, `{"response":{"server":{"serverid":"s1"}}}`)
		default:
			fmt.Fprint(w, `{"response":{"status":{"code":200},"future":{"id":9007199254740993123,"password":"must-hide"}}}`)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}
func (f *fixture) exchanges() []exchange {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]exchange(nil), f.requests...)
}
func (f *fixture) run(args ...string) (int, string, string) {
	return f.runWith(context.Background(), nil, "", append(args, "--json")...)
}
func (f *fixture) runWith(ctx context.Context, env map[string]string, input string, args ...string) (int, string, string) {
	var out, err bytes.Buffer
	a := newApp(strings.NewReader(input), &out, &err, BuildInfo{"test", "commit", "sdk"})
	a.testBaseURL = f.server.URL
	a.getenv = func(name string) string {
		if env != nil {
			return env[name]
		}
		switch name {
		case "ELG_PROJECT":
			return "CLfixture"
		case "ELG_API_KEY":
			return "fixture-secret"
		}
		return ""
	}
	args = append(args, "--config", filepath.Join(f.t.TempDir(), "config.json"))
	exit := a.execute(ctx, args)
	return exit, out.String(), err.String()
}
func (f *fixture) success(exit int, out, err string) map[string]any {
	return success(f.t, exit, out, err)
}
func success(t *testing.T, exit int, out, err string) map[string]any {
	t.Helper()
	if exit != 0 || err != "" {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, out, err)
	}
	var v map[string]any
	if e := json.Unmarshal([]byte(out), &v); e != nil {
		t.Fatal(e)
	}
	if v["schema_version"] != float64(1) {
		t.Fatal("missing schema version")
	}
	return v
}
func errorCode(t *testing.T, exit int, out, err string, wantExit int, wantCode string) {
	t.Helper()
	if exit != wantExit || out != "" {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, out, err)
	}
	var v struct {
		SchemaVersion int `json:"schema_version"`
		Error         struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	d := json.NewDecoder(strings.NewReader(err))
	if e := d.Decode(&v); e != nil {
		t.Fatal(e)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		t.Fatal("multiple error documents or usage text")
	}
	if v.Error.Code != wantCode {
		t.Fatalf("code=%s want=%s", v.Error.Code, wantCode)
	}
}
func TestEmailPresenceExactLookupAndUnits(t *testing.T) {
	f := newFixture(t, nil)
	f.success(f.run("email", "account", "update", "a@example.com", "--antispam-level", "0", "--autorespond=false", "--autorespond-message="))
	want := map[string]any{"emailaccount": "a@example.com", "antispamlevel": float64(0), "autorespond": "no", "autorespondmessage": ""}
	if !reflect.DeepEqual(f.exchanges()[0].body, want) {
		t.Fatalf("body=%v", f.exchanges()[0].body)
	}
	v := f.success(f.run("email", "account", "get", "a@example.com"))
	if strings.Contains(fmt.Sprint(v), "must-hide") {
		t.Fatal("secret exposed")
	}
	exit, out, err := f.run("email", "account", "get", "info@example.com")
	errorCode(t, exit, out, err, 4, "not_found")
	exit, out, err = f.run("email", "alias", "delete", "a@example.com", "--yes")
	errorCode(t, exit, out, err, 4, "not_found")
	v = f.success(f.run("email", "account", "quota", "a@example.com"))
	q := v["data"].(map[string]any)
	if q["usedquotainmib"] != float64(1024) || q["quotaingib"] != float64(2) {
		t.Fatal("quota units changed")
	}
}
func TestOverviewPaginationAndEmptyLists(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		page := 1
		if strings.HasSuffix(r.URL.Path, "/2") {
			page = 2
		}
		if !strings.Contains(r.URL.EscapedPath(), "/filter/a%2Fb") {
			t.Error("filter not retained or escaped")
		}
		fmt.Fprintf(w, `{"response":{"overview":{"summary":{"accounts":12},"domains":[{"domainname":"%d.example"}],"meta":{"page":%d,"total":2,"perpage":1}}}}`, page, page)
	})
	v := f.success(f.run("email", "overview", "--filter", "a/b"))
	if len(f.exchanges()) != 2 || v["meta"].(map[string]any)["pages_fetched"] != float64(2) {
		t.Fatal("pagination incomplete")
	}
	if v["data"].(map[string]any)["summary"].(map[string]any)["accounts"] != float64(12) {
		t.Fatal("summary aggregated incorrectly")
	}
	v = f.success(f.run("email", "overview", "--filter", "a/b", "--page", "2"))
	if v["meta"].(map[string]any)["complete"] != false {
		t.Fatal("one page labelled complete")
	}
	f2 := newFixture(t, nil)
	v = f2.success(f2.run("email", "overview"))
	if !reflect.DeepEqual(v["data"].(map[string]any)["domains"], []any{}) {
		t.Fatal("empty domains not []")
	}
	v = f2.success(f2.run("server", "list"))
	if !reflect.DeepEqual(v["data"], []any{}) {
		t.Fatal("empty list not []")
	}
}
func TestMalformedPaginationNeverSilentlyTruncates(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"response":{"overview":{"domains":[{"domainname":"repeat.example"}],"meta":{"page":1,"total":2,"perpage":1}}}}`)
	})
	exit, out, err := f.run("email", "overview")
	errorCode(t, exit, out, err, 1, "application_error")
}
func TestPasswordPreflightAndRedaction(t *testing.T) {
	f := newFixture(t, nil)
	path := filepath.Join(t.TempDir(), "password")
	f.success(f.run("email", "account", "reset-password", "a@example.com", "--yes", "--non-interactive", "--secret-output-file", path))
	b, e := os.ReadFile(path)
	if e != nil || string(b) != "new secret with spaces " {
		t.Fatalf("secret bytes: %q %v", b, e)
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatal("secret file is not private")
	}
	n := len(f.exchanges())
	exit, out, err := f.run("email", "account", "reset-password", "a@example.com", "--yes", "--secret-output-file", path)
	errorCode(t, exit, out, err, 2, "invalid_input")
	if len(f.exchanges()) != n+1 {
		t.Fatal("reset happened despite failed preflight")
	}
	exit, out, err = f.run("email", "account", "create", "a@example.com", "--non-interactive")
	errorCode(t, exit, out, err, 2, "invalid_input")
	v := f.success(f.run("email", "account", "create", "a@example.com", "--show-secrets"))
	if v["data"].(map[string]any)["password"] != "new secret with spaces " {
		t.Fatal("explicit display did not return secret")
	}
	last := f.exchanges()[len(f.exchanges())-1]
	if last.path != "/email/createaccount" {
		t.Fatal("wrong endpoint")
	}
	if _, ok := last.body["password"]; ok {
		t.Fatal("undocumented password supplied")
	}
	n = len(f.exchanges())
	f.success(f.run("email", "account", "create", "a@example.com", "--dry-run", "--secret-output-file", filepath.Join(t.TempDir(), "unused")))
	if len(f.exchanges()) != n {
		t.Fatal("dry-run mutated")
	}
}
func TestConfirmationAndDryRun(t *testing.T) {
	f := newFixture(t, nil)
	exit, out, err := f.run("server", "stop", "s1", "--non-interactive")
	errorCode(t, exit, out, err, 8, "confirmation_required")
	if len(f.exchanges()) != 0 {
		t.Fatal("unconfirmed mutation")
	}
	f.success(f.run("server", "stop", "s1", "--dry-run"))
	if len(f.exchanges()) != 0 {
		t.Fatal("dry-run sent mutation")
	}
	f.success(f.run("email", "alias", "delete", "info@example.com", "--yes"))
	if f.exchanges()[len(f.exchanges())-1].body["email"] != "info@example.com" {
		t.Fatal("delete body")
	}
}
func TestGenericPrecisionRedactionAndEndpointProtection(t *testing.T) {
	f := newFixture(t, nil)
	exit, out, err := f.run("api", "call", "unknown/read", "--method", "GET")
	errorCode(t, exit, out, err, 8, "confirmation_required")
	_, out, _ = f.run("api", "call", "unknown/read", "--yes")
	if !strings.Contains(out, "9007199254740993123") || strings.Contains(out, "must-hide") {
		t.Fatalf("raw data damaged: %s", out)
	}
	exit, out, err = f.run("api", "call", "email/quota", "--data-file", "-", "--json")
	errorCode(t, exit, out, err, 2, "invalid_input")
}
func TestGenericStdinAndBadEndpoints(t *testing.T) {
	f := newFixture(t, nil)
	exit, out, err := f.runWith(context.Background(), nil, `{"emailaccount":"a@example.com"}`, "api", "call", "email/quota", "--data-file", "-", "--json")
	success(t, exit, out, err)
	if f.exchanges()[0].method != "POST" || f.exchanges()[0].body["emailaccount"] != "a@example.com" {
		t.Fatal("stdin body")
	}
	for _, ep := range []string{"https://evil.test/x", "//evil.test/x", "../email/quota", "email/quota?x=1"} {
		exit, out, err = f.run("api", "call", ep, "--yes")
		errorCode(t, exit, out, err, 2, "invalid_input")
	}
	exit, out, err = f.runWith(context.Background(), nil, `{} {}`, "api", "call", "email/quota", "--data-file", "-", "--json")
	errorCode(t, exit, out, err, 2, "invalid_input")
}
func TestParserErrorsAndOutputSelection(t *testing.T) {
	f := newFixture(t, nil)
	for _, args := range [][]string{{"--json", "--not-a-flag"}, {"email", "account", "get", "--json"}, {"email", "account", "update", "a@example.com", "--antispam-level", "bad", "--json"}, {"--json", "unknown"}} {
		exit, out, err := f.run(args...)
		errorCode(t, exit, out, err, 2, "invalid_input")
	}
	exit, out, err := f.runWith(context.Background(), nil, "", "version")
	if exit != 0 || err != "" || strings.HasPrefix(out, "{") {
		t.Fatal("human default output ignored")
	}
	f.success(f.run("version", "--pretty"))
}
func TestAPIExitCodesAndNoPartialOutput(t *testing.T) {
	for _, tc := range []struct {
		status, exit int
		code         string
	}{{400, 2, "invalid_input"}, {401, 3, "authentication_error"}, {403, 3, "authentication_error"}, {404, 4, "not_found"}, {409, 5, "conflict"}, {429, 7, "rate_limit"}, {500, 1, "api_error"}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			f := newFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, "not-json") })
			exit, out, err := f.run("server", "list")
			errorCode(t, exit, out, err, tc.exit, tc.code)
		})
	}
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "not-json") })
	exit, out, err := f.run("server", "list")
	errorCode(t, exit, out, err, 1, "application_error")
}
func TestReadOnlyPOSTRetriesAndNoMutationRetry(t *testing.T) {
	attempts := 0
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(503)
			fmt.Fprint(w, `{}`)
		} else {
			fmt.Fprint(w, `{"response":{"quota":{"quotaingib":2}}}`)
		}
	})
	f.success(f.run("email", "account", "quota", "a@example.com"))
	if attempts != 2 {
		t.Fatal("read-only POST not retried")
	}
	attempts = 0
	exit, out, err := f.run("server", "start", "s1")
	errorCode(t, exit, out, err, 1, "api_error")
	if attempts != 1 {
		t.Fatal("mutation was retried")
	}
}
func TestCommandDeadlineAndInterruption(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	})
	exit, out, err := f.run("server", "list", "--timeout", "10ms")
	errorCode(t, exit, out, err, 6, "transport_error")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	exit, out, err = f.runWith(ctx, nil, "", "server", "list", "--json")
	errorCode(t, exit, out, err, 130, "interrupted")
}
func TestProfilePrecedenceAndMissingConfiguredKey(t *testing.T) {
	f := newFixture(t, nil)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Config{DefaultProfile: "default", Profiles: map[string]config.Profile{"default": {Project: "CLdefault", APIKeyEnv: "DEFAULT_KEY"}, "selected": {Project: "CLselected", APIKeyEnv: "SELECTED_KEY"}}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	run := func(env map[string]string, args ...string) (int, string, string) {
		var out, err bytes.Buffer
		a := newApp(strings.NewReader(""), &out, &err, BuildInfo{Version: "test"})
		a.testBaseURL = f.server.URL
		a.getenv = func(k string) string { return env[k] }
		return a.execute(context.Background(), append(args, "--config", path, "--json")), out.String(), err.String()
	}
	check := func(exit int, out, err string) map[string]any { return success(t, exit, out, err) }
	env := map[string]string{"ELG_PROFILE": "selected", "ELG_PROJECT": "CLenv", "SELECTED_KEY": "fixture-secret", "ELG_API_KEY": "wrong-key", "DEFAULT_KEY": "fixture-secret"}
	v := check(run(env, "server", "list", "--project", "CLflag"))
	if v["meta"].(map[string]any)["project"] != "CLflag" {
		t.Fatal("flag project lost")
	}
	v = check(run(env, "server", "list"))
	if v["meta"].(map[string]any)["project"] != "CLenv" {
		t.Fatal("environment project lost")
	}
	delete(env, "ELG_PROJECT")
	v = check(run(env, "server", "list"))
	if v["meta"].(map[string]any)["project"] != "CLselected" {
		t.Fatal("profile project lost")
	}
	v = check(run(env, "server", "list", "--profile", "default"))
	if v["meta"].(map[string]any)["project"] != "CLdefault" {
		t.Fatal("explicit profile lost")
	}
	delete(env, "SELECTED_KEY")
	exit, out, err := run(env, "server", "list")
	errorCode(t, exit, out, err, 2, "invalid_input")
	if strings.Contains(err, "wrong-key") {
		t.Fatal("credential exposed")
	}
	check(run(env, "config", "show"))
}
func TestDNSAndServerWrites(t *testing.T) {
	f := newFixture(t, nil)
	f.success(f.run("dns", "record", "create", "example.com", "--type", "A", "--host", "www", "--data", "192.0.2.1", "--ttl", "60"))
	if f.exchanges()[0].path != "/domain/addrecord" || f.exchanges()[0].body["ttl"] != float64(60) {
		t.Fatal("DNS exchange")
	}
	f.success(f.run("dns", "record", "update", "12", "--host="))
	if f.exchanges()[1].body["host"] != "@" {
		t.Fatal("empty host must select the zone apex")
	}
	f.success(f.run("server", "start", "s1"))
	f.success(f.run("server", "stop", "s1", "--yes", "--type", "hard"))
	f.success(f.run("server", "delete", "s1", "--yes", "--keep-ip"))
	if f.exchanges()[3].body["type"] != "hard" || f.exchanges()[4].body["keepip"] != true {
		t.Fatal("server lifecycle body")
	}
	f.success(f.run("server", "update", "s1", "--description="))
	if f.exchanges()[5].body["description"] != "" {
		t.Fatal("description clearing lost")
	}
	path := filepath.Join(t.TempDir(), "cloud-config")
	_ = os.WriteFile(path, []byte("#cloud-config\n"), 0600)
	f.success(f.run("server", "create", "--hostname", "test", "--datacenter", "Falkenberg", "--platform", "KVM", "--template", "debian-id", "--cpu-cores", "2", "--memory-mib", "2048", "--disk-gib", "20", "--cloud-config-file", path))
	if f.exchanges()[6].body["cloudconfig"] != "#cloud-config\n" || f.exchanges()[6].body["templatename"] != "debian-id" {
		t.Fatal("provisioning exchange")
	}
}

func TestDNSApexHostEncoding(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		host string
		sent bool
	}{
		{"create explicit apex", []string{"dns", "record", "create", "example.com", "--type", "TXT", "--data", "test", "--host", "@"}, "@", true},
		{"create explicit empty apex", []string{"dns", "record", "create", "example.com", "--type", "TXT", "--data", "test", "--host="}, "@", true},
		{"create named host", []string{"dns", "record", "create", "example.com", "--type", "TXT", "--data", "test", "--host", "test"}, "test", true},
		{"update explicit empty apex", []string{"dns", "record", "update", "12", "--host="}, "@", true},
		{"update named host", []string{"dns", "record", "update", "12", "--host", "test"}, "test", true},
		{"update omitted host", []string{"dns", "record", "update", "12", "--data", "test"}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, nil)
			f.success(f.run(tc.args...))
			host, sent := f.exchanges()[0].body["host"]
			if sent != tc.sent || (sent && host != tc.host) {
				t.Fatalf("host=%v sent=%v, want host=%q sent=%v", host, sent, tc.host, tc.sent)
			}
		})
	}
}

func TestDNSRecordTypeUpdateRejectedBeforeMutation(t *testing.T) {
	f := newFixture(t, nil)
	exit, out, err := f.run("dns", "record", "update", "12", "--type", "AAAA", "--data", "2001:db8::1")
	errorCode(t, exit, out, err, 2, "invalid_input")
	if len(f.exchanges()) != 0 {
		t.Fatal("unsupported type change must not partially update record data")
	}
}
