package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	glesys "github.com/glesys/glesys-go/v8"
)

const maxResponseBytes = 16 << 20

// endpointPattern accepts only a relative service/operation, never a URL.
var endpointPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+/[A-Za-z0-9_-]+$`)

// rawRequest returns the complete provider envelope for endpoints the SDK has
// no typed method for. It shares the SDK client's HTTP client (retries,
// timeouts, no redirects). GET parameters, already validated as scalars by the
// caller, use the documented /name/value path convention.
func (a *app) rawRequest(method, endpoint string, params map[string]any) (json.RawMessage, error) {
	path := endpoint
	var body io.Reader = http.NoBody
	if method == http.MethodGet {
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			path += "/" + k + "/" + url.PathEscape(fmt.Sprint(params[k]))
		}
	} else if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(a.ctx, method, strings.TrimRight(a.baseURL, "/")+"/"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "elg/"+a.build.Version)
	req.SetBasicAuth(a.project, a.key)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("API response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode != http.StatusOK {
		var env struct {
			Response struct {
				Status struct {
					Code int    `json:"code"`
					Text string `json:"text"`
				} `json:"status"`
			} `json:"response"`
		}
		_ = json.Unmarshal(data, &env)
		return nil, &glesys.APIError{HTTPStatus: resp.StatusCode, ProviderStatus: env.Response.Status.Code, Message: strings.TrimSpace(env.Response.Status.Text), RetryAfter: resp.Header.Get("Retry-After")}
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("API returned invalid or empty JSON")
	}
	return data, nil
}
