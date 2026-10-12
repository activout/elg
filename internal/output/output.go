package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
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
	if format == "json" || format == "ndjson" {
		e := json.NewEncoder(&buf)
		e.SetEscapeHTML(false)
		if pretty && format == "json" {
			e.SetIndent("", "  ")
		}
		rows := []any{v}
		if format == "ndjson" {
			if list, ok := v.([]any); ok {
				rows = list
			}
		}
		for _, row := range rows {
			if err = e.Encode(Envelope{1, row, meta}); err != nil {
				return err
			}
		}
	} else if v == nil {
		if operation, ok := meta["operation"]; ok {
			fmt.Fprintf(&buf, "%s: %s\n", cell(operation), cell(meta["target"]))
		} else {
			fmt.Fprintln(&buf, "No result")
		}
	} else {
		table(&buf, v, "")
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
func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return "—"
	case string:
		q := strconv.Quote(x)
		return q[1 : len(q)-1]
	case []any:
		values := make([]string, len(x))
		for i, v := range x {
			values[i] = cell(v)
		}
		return strings.Join(values, ", ")
	case map[string]any:
		values := []string{}
		for _, k := range keys(x) {
			values = append(values, cell(k)+"="+cell(x[k]))
		}
		return strings.Join(values, ", ")
	default:
		return fmt.Sprint(v)
	}
}
func keys(m map[string]any) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}
func table(w io.Writer, v any, indent string) {
	t := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	defer t.Flush()
	switch x := v.(type) {
	case []any:
		if len(x) == 0 {
			fmt.Fprintln(t, indent+"No results")
			return
		}
		columns := map[string]any{}
		allObjects := true
		for _, r := range x {
			if m, ok := r.(map[string]any); ok {
				for k := range m {
					columns[k] = nil
				}
			} else {
				allObjects = false
			}
		}
		names := keys(columns)
		if !allObjects || len(names) == 0 {
			for _, r := range x {
				fmt.Fprintln(t, indent+cell(r))
			}
			return
		}
		headers := make([]string, len(names))
		for i, k := range names {
			headers[i] = cell(k)
		}
		fmt.Fprintln(t, indent+strings.Join(headers, "\t"))
		for _, r := range x {
			m, _ := r.(map[string]any)
			vals := make([]string, len(names))
			for i, k := range names {
				vals[i] = cell(m[k])
			}
			fmt.Fprintln(t, indent+strings.Join(vals, "\t"))
		}
	case map[string]any:
		if len(x) == 0 {
			fmt.Fprintln(t, indent+"No results")
			return
		}
		nested := []string{}
		header := false
		for _, k := range keys(x) {
			switch x[k].(type) {
			case map[string]any, []any:
				nested = append(nested, k)
				continue
			}
			if !header {
				fmt.Fprintln(t, indent+"FIELD\tVALUE")
				header = true
			}
			fmt.Fprintf(t, "%s%s\t%s\n", indent, cell(k), cell(x[k]))
		}
		_ = t.Flush()
		for _, k := range nested {
			fmt.Fprintf(w, "%s%s:\n", indent, cell(k))
			table(w, x[k], indent+"  ")
		}
	default:
		fmt.Fprintln(t, indent+cell(v))
	}
}
