package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}
type Envelope struct {
	SchemaVersion int            `json:"schema_version"`
	Data          any            `json:"data"`
	Meta          map[string]any `json:"meta"`
}

// Value uses JSON numbers so raw identifiers never pass through float64.
func Value(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var result any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	err = d.Decode(&result)
	return result, err
}
func secretKey(k string) bool {
	if strings.HasSuffix(strings.ToLower(k), "_env") {
		return false
	}
	k = strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(k))
	for _, s := range []string{"password", "secret", "token", "privatekey", "apikey", "accesskey", "authcode", "transfercode", "credential", "authorization"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return k == "key"
}
func Redact(v any, show bool, apiKey string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if secretKey(k) && (!show || strings.Contains(strings.ToLower(k), "authorization") || strings.Contains(strings.ToLower(k), "apikey") || strings.EqualFold(k, "api_key")) {
				x[k] = "[REDACTED]"
			} else {
				x[k] = Redact(val, show, apiKey)
			}
		}
		return x
	case []any:
		for i := range x {
			x[i] = Redact(x[i], show, apiKey)
		}
		return x
	case string:
		if apiKey != "" {
			x = strings.ReplaceAll(x, apiKey, "[REDACTED]")
		}
		if !show && strings.Contains(x, "PRIVATE KEY-----") {
			return "[REDACTED]"
		}
		return x
	default:
		return v
	}
}
func Write(w io.Writer, data any, meta map[string]any, format string, pretty, show bool, key string) error {
	v, err := Value(data)
	if err != nil {
		return err
	}
	v = Redact(v, show, key)
	var buf bytes.Buffer
	if format == "json" {
		e := json.NewEncoder(&buf)
		e.SetEscapeHTML(false)
		if pretty {
			e.SetIndent("", "  ")
		}
		if err = e.Encode(Envelope{1, v, meta}); err != nil {
			return err
		}
	} else if v == nil {
		fmt.Fprintf(&buf, "%v: %v\n", meta["operation"], meta["target"])
	} else {
		table(&buf, v)
	}
	_, err = w.Write(buf.Bytes())
	return err
}
func WriteError(w io.Writer, e Error, jsonMode bool) {
	if jsonMode {
		_ = json.NewEncoder(w).Encode(struct {
			SchemaVersion int   `json:"schema_version"`
			Error         Error `json:"error"`
		}{1, e})
	} else {
		fmt.Fprintln(w, "Error:", e.Message)
	}
}
func cell(v any) string { b, _ := json.Marshal(v); return string(b) }
func keys(m map[string]any) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}
func table(w io.Writer, v any) {
	t := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	defer t.Flush()
	switch x := v.(type) {
	case []any:
		if len(x) == 0 {
			fmt.Fprintln(t, "No results")
			return
		}
		columns := map[string]any{}
		for _, r := range x {
			if m, ok := r.(map[string]any); ok {
				for k := range m {
					columns[k] = nil
				}
			}
		}
		names := keys(columns)
		if len(names) == 0 {
			for _, r := range x {
				fmt.Fprintln(t, cell(r))
			}
			return
		}
		fmt.Fprintln(t, strings.Join(names, "\t"))
		for _, r := range x {
			m, _ := r.(map[string]any)
			vals := make([]string, len(names))
			for i, k := range names {
				vals[i] = cell(m[k])
			}
			fmt.Fprintln(t, strings.Join(vals, "\t"))
		}
	case map[string]any:
		fmt.Fprintln(t, "FIELD\tVALUE")
		for _, k := range keys(x) {
			fmt.Fprintf(t, "%s\t%s\n", k, cell(x[k]))
		}
	default:
		fmt.Fprintln(t, cell(v))
	}
}
