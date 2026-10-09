package abs

import (
	"encoding/json/v2"
	"net/http"
	"strings"
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
		url:        uri,
		key:        key,
		httpclient: http.DefaultClient,
	}
}

func (c *Client) get[T any](path string) (*T, error) {
	r, err := http.NewRequest(http.MethodGet, c.url+path, http.NoBody)
	if err != nil {
		return nil, err
	}
	return c.do[T](r)
}

func (c *Client) do[T any](req *http.Request) (*T, error) {
	req.Header.Add("Authorization", "Bearer "+c.key)
	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result := new(T)

	err = json.UnmarshalRead(resp.Body, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
