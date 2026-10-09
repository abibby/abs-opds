package abs

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	url string
	key string

	httpclient *http.Client
}

func New(uri, key string) *Client {
	uri = strings.TrimSuffix(uri, "/")
	uri = strings.TrimSuffix(uri, "/api")
	uri += "/api"
	return &Client{
		url: uri,
		key: key,
		httpclient: &http.Client{
			Timeout: 5 * time.Minute,
			// Downloads must not redirect clients or credentials to another server.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *Client) get[T any](ctx context.Context, path string) (*T, error) {
	return c.requestJSON[T](ctx, http.MethodGet, path, nil)
}

func (c *Client) post[T any](ctx context.Context, path string, body io.Reader) (*T, error) {
	return c.requestJSON[T](ctx, http.MethodPost, path, body)
}

func (c *Client) requestJSON[T any](ctx context.Context, method, path string, body io.Reader) (*T, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.url+path, body)
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	result := new(T)
	if err := json.UnmarshalRead(resp.Body, result); err != nil {
		return nil, fmt.Errorf("decode Audiobookshelf response: %w", err)
	}
	return result, nil
}

// HTTPError preserves upstream status without exposing its response body or token.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("Audiobookshelf returned HTTP %d", e.StatusCode)
}

// getResponse opens an authenticated API response. The caller must close its body.
// path is an internally constructed API path, never a URL supplied by a client.
func (c *Client) getResponse(ctx context.Context, path string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+path, http.NoBody)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := headers.Get(name); value != "" {
			req.Header.Set(name, value)
		}
	}
	return c.do(req)
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.key)
	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	if (resp.StatusCode < 200 || resp.StatusCode >= 300) && resp.StatusCode != http.StatusNotModified {
		resp.Body.Close()
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}
	return resp, nil
}
