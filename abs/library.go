package abs

import (
	"context"
	"gosalusa.com/option"
	"net/url"
)

type Library struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Folders      []Folder        `json:"folders"`
	DisplayOrder int             `json:"displayOrder"`
	Icon         string          `json:"icon"`
	MediaType    string          `json:"mediaType"`
	Provider     string          `json:"provider"`
	Settings     LibrarySettings `json:"settings"`
	CreatedAt    int64           `json:"createdAt"`
	LastUpdate   int64           `json:"lastUpdate"`
}

type Folder struct {
	ID        string `json:"id"`
	FullPath  string `json:"fullPath"`
	LibraryID string `json:"libraryId"`
}

type LibrarySettings struct {
	CoverAspectRatio          int                   `json:"coverAspectRatio"`
	DisableWatcher            bool                  `json:"disableWatcher"`
	SkipMatchingMediaWithAsin bool                  `json:"skipMatchingMediaWithAsin"`
	SkipMatchingMediaWithIsbn bool                  `json:"skipMatchingMediaWithIsbn"`
	AutoScanCronExpression    option.Option[string] `json:"autoScanCronExpression"`
}

type GetAllLibrariesResponse struct {
	Libraries []Library `json:"libraries"`
}

func (c *Client) GetAllLibraries() (*GetAllLibrariesResponse, error) {
	return c.GetLibraries(context.Background())
}

func (c *Client) GetLibraries(ctx context.Context) (*GetAllLibrariesResponse, error) {
	return get[GetAllLibrariesResponse](ctx, c, "/libraries")
}

func (c *Client) GetLibrary(ctx context.Context, id string) (*Library, error) {
	return get[Library](ctx, c, "/libraries/"+url.PathEscape(id))
}
