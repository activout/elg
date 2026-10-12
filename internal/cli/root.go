package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/activout/elg/internal/config"
	"github.com/activout/elg/internal/output"
	"github.com/activout/elg/internal/secrets"
	glesys "github.com/glesys/glesys-go/v8"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type BuildInfo struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	SDKVersion string `json:"sdk_version"`
}
type failure struct {
	code    string
	message string
	exit    int
	status  int
}

func (e *failure) Error() string    { return e.message }
func invalid(message string) error  { return &failure{"invalid_input", message, 2, 0} }
func notFound(message string) error { return &failure{"not_found", message, 4, 404} }

type app struct {
	root                                                                  *cobra.Command
	in                                                                    io.Reader
	out, err                                                              io.Writer
	getenv                                                                func(string) string
	profile, project, configPath, format, timeoutText, baseURL            string
	json, ndjson, pretty, yes, nonInteractive, dryRun, debug, showSecrets bool
	client                                                                *glesys.Client
	key                                                                   string
	requestSecrets                                                        []string
	meta                                                                  map[string]any
	ctx                                                                   context.Context
	cancel                                                                context.CancelFunc
	build                                                                 BuildInfo
	inputTTY, outputTTY                                                   bool
	testBaseURL                                                           string
}

func newApp(in io.Reader, out, err io.Writer, build BuildInfo) *app {
	a := &app{in: in, out: out, err: err, build: build, getenv: os.Getenv}
	if f, ok := in.(*os.File); ok {
		a.inputTTY = term.IsTerminal(int(f.Fd()))
	}
	if f, ok := out.(*os.File); ok {
		a.outputTTY = term.IsTerminal(int(f.Fd()))
	}
	r := &cobra.Command{Use: "elg", Short: "Independent administration CLI for GleSYS", SilenceUsage: true, SilenceErrors: true}
	a.root = r
	r.SetIn(in)
	r.SetOut(out)
	r.SetErr(err)
	r.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return invalid(err.Error()) })
	f := r.PersistentFlags()
	f.StringVar(&a.profile, "profile", "", "Named profile (ELG_PROFILE)")
	f.StringVar(&a.project, "project", "", "Project ID (ELG_PROJECT; ELG_PROJECT_ID fallback)")
	f.StringVar(&a.configPath, "config", "", "Configuration file (ELG_CONFIG)")
	f.BoolVar(&a.json, "json", false, "Output a JSON result envelope")
	f.BoolVar(&a.ndjson, "ndjson", false, "Output one compact JSON envelope per list item")
	r.MarkFlagsMutuallyExclusive("json", "ndjson")
	f.BoolVar(&a.pretty, "pretty", false, "Indent JSON results")
	f.BoolVar(&a.yes, "yes", false, "Confirm destructive operations")
	f.BoolVar(&a.nonInteractive, "non-interactive", false, "Never prompt")
	f.BoolVar(&a.dryRun, "dry-run", false, "Validate and show a redacted plan without mutation")
	f.BoolVar(&a.debug, "debug", false, "Log operation names to stderr, without payloads")
	f.BoolVar(&a.showSecrets, "show-secrets", false, "Deliberately include result secrets; never authentication credentials")
	f.StringVar(&a.timeoutText, "timeout", "", "Whole-command timeout (ELG_TIMEOUT; default 30s)")
	r.AddCommand(a.configCommands(), a.emailCommands(), a.dnsCommands(), a.serverCommands(), a.apiCommands())
	r.AddCommand(&cobra.Command{Use: "version", Short: "Show build and SDK versions", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		if err := a.settings(); err != nil {
			return err
		}
		return a.write(a.build, map[string]any{"command": "version"})
	}})
	r.RunE = func(c *cobra.Command, args []string) error { return invalid("a command is required; use elg --help") }
	return a
}

func Run(ctx context.Context, args []string, in io.Reader, out, err io.Writer, build BuildInfo) int {
	a := newApp(in, out, err, build)
	return a.execute(ctx, args)
}
func (a *app) execute(ctx context.Context, args []string) int {
	a.root.SetArgs(args)
	e := a.root.ExecuteContext(ctx)
	if a.cancel != nil {
		defer a.cancel()
	}
	if e == nil {
		if a.debug && a.client != nil {
			fmt.Fprintf(a.err, "elg: project=%s command=%s\n", strings.ReplaceAll(a.project, a.key, "[REDACTED]"), a.meta["command"])
		}
		return 0
	}
	code, status, exit := classify(e)
	if code == "application_error" && a.client == nil {
		code = "invalid_input"
		exit = 2
	}
	message := e.Error()
	if code == "transport_error" || code == "interrupted" {
		message = "Transport failure or command deadline exceeded"
		if code == "interrupted" {
			message = "Interrupted"
		}
	}
	if a.key != "" {
		message = strings.ReplaceAll(message, a.key, "[REDACTED]")
	}
	for _, secret := range a.requestSecrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	output.WriteError(a.err, output.Error{Code: code, Message: message, HTTPStatus: status}, a.machineErrors(args))
	return exit
}
func classify(err error) (string, int, int) {
	var f *failure
	if errors.As(err, &f) {
		return f.code, f.status, f.exit
	}
	if errors.Is(err, context.Canceled) {
		return "interrupted", 0, 130
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "transport_error", 0, 6
	}
	var api *glesys.APIError
	if errors.As(err, &api) {
		status := api.HTTPStatus
		if api.ProviderStatus >= 400 {
			status = api.ProviderStatus
		}
		switch status {
		case 400, 422:
			return "invalid_input", api.HTTPStatus, 2
		case 401, 403:
			return "authentication_error", api.HTTPStatus, 3
		case 404:
			return "not_found", api.HTTPStatus, 4
		case 409, 412:
			return "conflict", api.HTTPStatus, 5
		case 429:
			return "rate_limit", api.HTTPStatus, 7
		}
		return "api_error", api.HTTPStatus, 1
	}
	var n net.Error
	if errors.As(err, &n) {
		return "transport_error", 0, 6
	}
	return "application_error", 0, 1
}

// Inspect explicit flags even when argument parsing failed before settings ran.
func (a *app) machineErrors(args []string) bool {
	jsonMode, ndjsonMode := false, false
	for _, s := range args {
		if s == "--" {
			break
		}
		switch s {
		case "--json", "--json=true":
			jsonMode = true
		case "--json=false":
			jsonMode = false
		case "--ndjson", "--ndjson=true":
			ndjsonMode = true
		case "--ndjson=false":
			ndjsonMode = false
		}
	}
	return jsonMode || ndjsonMode
}
func (a *app) settings() error {
	a.format = "table"
	if a.json {
		a.format = "json"
	}
	if a.ndjson {
		a.format = "ndjson"
	}
	if a.pretty && !a.json {
		return invalid("--pretty requires --json")
	}
	if !a.root.PersistentFlags().Changed("config") {
		a.configPath = a.getenv("ELG_CONFIG")
	}
	if a.configPath == "" {
		p, err := config.Path()
		if err != nil {
			return invalid(err.Error())
		}
		a.configPath = p
	}
	return nil
}

func (a *app) connect(c *cobra.Command) error {
	if err := a.settings(); err != nil {
		return err
	}
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return invalid(err.Error())
	}
	if !a.root.PersistentFlags().Changed("profile") {
		a.profile = a.getenv("ELG_PROFILE")
	}
	if a.profile == "" {
		a.profile = cfg.DefaultProfile
	}
	p := config.Profile{}
	if a.profile != "" {
		var exists bool
		p, exists = cfg.Profiles[a.profile]
		if !exists {
			return invalid("profile was not found")
		}
	}
	if !a.root.PersistentFlags().Changed("project") {
		a.project = a.getenv("ELG_PROJECT")
		if a.project == "" {
			a.project = a.getenv("ELG_PROJECT_ID")
		}
		if a.project == "" {
			a.project = p.Project
		}
	}
	if a.project == "" {
		return invalid("project is required: use --project, ELG_PROJECT or a profile")
	}
	variable := p.APIKeyEnv
	if variable == "" {
		variable = "ELG_API_KEY"
	}
	a.key = a.getenv(variable)
	if a.key == "" {
		return invalid("credential environment variable " + variable + " is missing; no fallback key was used")
	}
	duration := a.timeoutText
	if !a.root.PersistentFlags().Changed("timeout") {
		duration = a.getenv("ELG_TIMEOUT")
	}
	if duration == "" {
		duration = "30s"
	}
	timeout, err := time.ParseDuration(duration)
	if err != nil || timeout <= 0 {
		return invalid("timeout must be a positive duration, such as 30s")
	}
	a.ctx, a.cancel = context.WithTimeout(c.Context(), timeout)
	a.baseURL = p.APIBaseURL
	if a.baseURL == "" {
		a.baseURL = "https://api.glesys.com"
	}
	if err = validateBaseURL(a.baseURL); err != nil {
		return err
	}
	if a.testBaseURL != "" {
		a.baseURL = a.testBaseURL
	}
	a.client = glesys.NewClient(a.project, a.key, "elg/"+a.build.Version)
	if err = a.client.SetBaseURL(a.baseURL); err != nil {
		return invalid(err.Error())
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = timeout
	a.client.SetHTTPClient(&http.Client{Transport: retryTransport{transport}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }})
	a.meta = map[string]any{"command": strings.TrimPrefix(c.CommandPath(), "elg "), "project": a.project}

	return nil
}
func validateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return invalid("API base URL must be an HTTPS origin without credentials, path, query or fragment")
	}
	return nil
}
func (a *app) write(data any, meta map[string]any) error {
	return output.Write(a.out, data, meta, a.format, a.pretty, a.showSecrets && !a.dryRun, a.key)
}
func (a *app) result(data any) error { return a.write(data, a.meta) }
func (a *app) operation(operation, target string) error {
	a.meta["operation"] = operation
	a.meta["target"] = target
	return a.result(nil)
}
func (a *app) plan(operation, target string, values any) error {
	return a.result(map[string]any{"dry_run": true, "operation": operation, "target": target, "changes": values, "provider_validated": false})
}
func (a *app) confirm(c *cobra.Command, target, consequence string) error {
	if a.yes {
		return nil
	}
	if a.nonInteractive || !a.inputTTY || !a.outputTTY {
		return &failure{"confirmation_required", "confirmation requires --yes in non-interactive execution", 8, 0}
	}
	fmt.Fprintf(a.err, "Project %s: %s (%s). Type yes to confirm: ", a.project, target, consequence)
	line, err := a.readLine(a.ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(line) != "yes" {
		return &failure{"confirmation_declined", "operation was not confirmed", 8, 0}
	}
	return nil
}
func (a *app) readLine(ctx context.Context) (string, error) {
	type answer struct {
		s   string
		err error
	}
	ch := make(chan answer, 1)
	go func() { s, e := bufio.NewReader(a.in).ReadString('\n'); ch <- answer{s, e} }()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case v := <-ch:
		return v.s, v.err
	}
}
func (a *app) secretDestination(c *cobra.Command, path string) (*secrets.Destination, error) {
	if path != "" {
		d, err := secrets.Reserve(path)
		if err != nil {
			return nil, invalid(err.Error())
		}
		return d, nil
	}
	if a.showSecrets {
		return nil, nil
	}
	if a.nonInteractive || !a.inputTTY || !a.outputTTY {
		return nil, invalid("password generation requires --secret-output-file or deliberate --show-secrets")
	}
	fmt.Fprint(a.err, "A password may be generated. Type show to display it, or enter a new secret file path: ")
	answer, err := a.readLine(a.ctx)
	if err != nil {
		return nil, err
	}
	answer = strings.TrimSpace(answer)
	if answer == "show" {
		a.showSecrets = true
		return nil, nil
	}
	if answer == "" {
		return nil, invalid("a secret destination is required")
	}
	d, err := secrets.Reserve(answer)
	if err != nil {
		return nil, invalid(err.Error())
	}
	return d, nil
}
func deliver(d *secrets.Destination, secret string) error {
	if secret == "" {
		return &failure{"secret_delivery_failed", "mutation succeeded but the API returned no password; do not repeat the mutation automatically", 1, 0}
	}
	if d != nil {
		if err := d.Write(secret); err != nil {
			return &failure{"secret_delivery_failed", "mutation succeeded but secret delivery failed; do not retry automatically", 1, 0}
		}
	}
	return nil
}
func mutationError(err error) error {
	if err == nil {
		return nil
	}
	code, _, _ := classify(err)
	if code == "transport_error" || code == "interrupted" || code == "application_error" {
		_, status, exit := classify(err)
		return &failure{"mutation_outcome_unknown", "mutation outcome is unknown because the response could not be confirmed; inspect the resource before retrying", exit, status}
	}
	return err
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func validateID(id string) error {
	if !idPattern.MatchString(id) {
		return invalid("invalid resource ID")
	}
	return nil
}
func validateDomain(domain string) error {
	if domain == "" || domain == "." || domain == ".." || strings.ContainsAny(domain, "/\\%?#:@<> \t\r\n") {
		return invalid("invalid domain name")
	}
	return nil
}
func addressDomain(address string) (string, error) {
	i := strings.LastIndexByte(address, '@')
	if i < 1 || i == len(address)-1 || strings.ContainsAny(address, "\r\n<>") {
		return "", invalid("expected an email address with local part and domain")
	}
	d := address[i+1:]
	return d, validateDomain(d)
}
func command(use, description string, args cobra.PositionalArgs, run func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{Use: use, Short: description, Args: func(c *cobra.Command, a []string) error {
		if err := args(c, a); err != nil {
			return invalid(err.Error())
		}
		return nil
	}, RunE: run}
}
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
