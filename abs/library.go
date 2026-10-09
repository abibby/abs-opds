package abs

import (
	"context"
	"net/url"
)

type Library struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
}

func (c *Client) GetLibrary(ctx context.Context, id string) (*Library, error) {
	return c.get[Library](ctx, "/libraries/"+url.PathEscape(id))
}
