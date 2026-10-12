package cli

import (
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Only verified read operations are retried, including read-only POSTs.
var reads = map[string]bool{
	"api/listfunctions": true,
	"email/overview":    true, "email/list": true, "email/quota": true, "email/costs": true,
	"domain/list": true, "domain/details": true, "domain/export": true, "domain/listrecords": true,
	"server/list": true, "server/details": true, "server/templates": true,
}

type retryTransport struct{ base http.RoundTripper }

func endpoint(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}
func retryDelay(header string, attempt int) time.Duration {
	if n, err := strconv.Atoi(header); err == nil && n >= 0 {
		return time.Duration(min(n, 86400)) * time.Second
	}
	if date, err := http.ParseTime(header); err == nil {
		return max(time.Until(date), 0)
	}
	return time.Duration(100*(1<<attempt)+rand.IntN(100)) * time.Millisecond
}
func (t retryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		defer r.Body.Close()
	}
	for attempt := 0; ; attempt++ {
		request := r.Clone(r.Context())
		if r.Body != nil && r.GetBody != nil {
			b, err := r.GetBody()
			if err != nil {
				return nil, err
			}
			request.Body = b
		}
		resp, err := t.base.RoundTrip(request)
		retry := err != nil || (resp != nil && (resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504))
		if !reads[endpoint(r.URL.Path)] || !retry || attempt == 2 || r.Context().Err() != nil {
			return resp, err
		}
		header := ""
		if resp != nil {
			header = resp.Header.Get("Retry-After")
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			_ = resp.Body.Close()
		}
		timer := time.NewTimer(retryDelay(header, attempt))
		select {
		case <-r.Context().Done():
			timer.Stop()
			return nil, r.Context().Err()
		case <-timer.C:
		}
	}
}
