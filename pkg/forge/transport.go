package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DoJSON performs one JSON-over-HTTP API call shared by every outbound
// AdminClient (github/gitlab/forgejo). body (when non-nil) is
// JSON-encoded and Content-Type set; out (when non-nil and the status is
// 2xx) is JSON-decoded. The per-driver setHeaders callback applies the
// auth scheme + any provider-specific headers (the token is never placed
// in the URL, so it cannot leak through error strings). errPrefix names
// the provider in wrapped errors ("gitlab"/"github"/"forgejo").
//
// The drivers differ only in (url, setHeaders, errPrefix); this body —
// marshal, request, decode, drain-on-error — was previously copy-pasted
// three times.
func DoJSON(ctx context.Context, client *http.Client, method, url, errPrefix string, setHeaders func(*http.Request), body, out any) (int, error) {
	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("%s: marshal body: %w", errPrefix, err)
		}
		reqBody = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return 0, err
	}
	if setHeaders != nil {
		setHeaders(req)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode/100 == 2 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("%s: decode response: %w", errPrefix, err)
		}
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	}
	return resp.StatusCode, nil
}

// StatusErr maps a non-2xx status to the appropriate forge sentinel,
// falling back to a "<prefix>: <op>: HTTP <code>" error. Shared by every
// AdminClient so the 401/403/404 mapping stays identical across providers.
func StatusErr(errPrefix, op string, code int) error {
	switch code {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrHookNotFound
	default:
		return fmt.Errorf("%s: %s: HTTP %d", errPrefix, op, code)
	}
}
