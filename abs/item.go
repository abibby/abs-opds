package abs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type Item struct {
	ID           string `json:"id"`
	LibraryID    string `json:"libraryId"`
	MediaType    string `json:"mediaType"`
	AddedAt      int64  `json:"addedAt"`
	UpdatedAt    int64  `json:"updatedAt"`
	IsMissing    bool   `json:"isMissing"`
	IsInvalid    bool   `json:"isInvalid"`
	Media        Book   `json:"media"`
	LibraryFiles []File `json:"libraryFiles"`
}

type Book struct {
	Metadata  BookMetadata `json:"metadata"`
	CoverPath string       `json:"coverPath"`
	EbookFile *File        `json:"ebookFile"`
}

type BookMetadata struct {
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Authors     []Author `json:"authors"`
	AuthorName  string   `json:"authorName"`
	Description string   `json:"description"`
	Publisher   string   `json:"publisher"`
	Language    string   `json:"language"`
	ISBN        string   `json:"isbn"`
	Genres      []string `json:"genres"`
	Series      []Series `json:"series"`
}

type Series struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sequence string `json:"sequence"`
}

type Author struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type File struct {
	Ino         string       `json:"ino"`
	EbookFormat string       `json:"ebookFormat"`
	Metadata    FileMetadata `json:"metadata"`
}

type FileMetadata struct {
	Filename string `json:"filename"`
	Ext      string `json:"ext"`
	Size     int64  `json:"size"`
}

func (f File) IsEPUB() bool {
	return strings.EqualFold(f.EbookFormat, "epub") ||
		strings.EqualFold(f.Metadata.Ext, ".epub") ||
		strings.EqualFold(path.Ext(f.Metadata.Filename), ".epub")
}

// EPUBs includes primary and supplementary files, without duplicating the primary.
func (i Item) EPUBs() []File {
	if i.MediaType != "book" || i.IsMissing || i.IsInvalid {
		return nil
	}
	var files []File
	seen := map[string]bool{}
	add := func(f File) {
		if f.Ino != "" && f.IsEPUB() && !seen[f.Ino] {
			files = append(files, f)
			seen[f.Ino] = true
		}
	}
	if i.Media.EbookFile != nil {
		add(*i.Media.EbookFile)
	}
	for _, f := range i.LibraryFiles {
		add(f)
	}
	return files
}

type Page[T any] struct {
	Results []T `json:"results"`
	Total   int `json:"total"`
}

// Listing metadata varies by filter (notably series can be an object instead
// of an array). Only IDs are needed before fetching this page's full metadata.
type ItemSummary struct {
	ID string `json:"id"`
}

type ItemsResponse = Page[ItemSummary]

type ItemQuery struct {
	Page     int
	Limit    int
	AuthorID string
	SeriesID string
	Recent   bool
}

func pageQuery(page, limit int, sort string) url.Values {
	return url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(limit)}, "sort": {sort}}
}

func (c *Client) GetLibraryItems(ctx context.Context, libraryID string, options ItemQuery) (*ItemsResponse, error) {
	if options.AuthorID != "" && options.SeriesID != "" {
		return nil, fmt.Errorf("Audiobookshelf supports only one category filter per request")
	}
	query := pageQuery(options.Page, options.Limit, "media.metadata.title")
	query.Set("minified", "1")
	if options.AuthorID != "" {
		query.Set("filter", "authors."+base64.StdEncoding.EncodeToString([]byte(options.AuthorID)))
	}
	if options.SeriesID != "" {
		query.Set("filter", "series."+base64.StdEncoding.EncodeToString([]byte(options.SeriesID)))
		query.Set("sort", "sequence")
	}
	if options.Recent {
		query.Set("sort", "addedAt")
		query.Set("desc", "1")
	}
	return get[ItemsResponse](ctx, c, "/libraries/"+url.PathEscape(libraryID)+"/items?"+query.Encode())
}

func (c *Client) GetLibraryAuthors(ctx context.Context, libraryID string, page, limit int) (*Page[Author], error) {
	return get[Page[Author]](ctx, c, "/libraries/"+url.PathEscape(libraryID)+"/authors?"+pageQuery(page, limit, "name").Encode())
}

func (c *Client) GetLibrarySeries(ctx context.Context, libraryID string, page, limit int) (*Page[Series], error) {
	return get[Page[Series]](ctx, c, "/libraries/"+url.PathEscape(libraryID)+"/series?"+pageQuery(page, limit, "name").Encode())
}

type SearchResponse struct {
	Books []struct {
		Item Item `json:"libraryItem"`
	} `json:"book"`
}

// SearchLibrary uses the native search endpoint, which supports a limit but no
// offset or total count. Results already contain expanded item metadata.
func (c *Client) SearchLibrary(ctx context.Context, libraryID, terms string, limit int) (*SearchResponse, error) {
	query := url.Values{"q": {terms}, "limit": {strconv.Itoa(limit)}}
	return get[SearchResponse](ctx, c, "/libraries/"+url.PathEscape(libraryID)+"/search?"+query.Encode())
}

func (c *Client) GetItem(ctx context.Context, id string) (*Item, error) {
	return get[Item](ctx, c, "/items/"+url.PathEscape(id))
}

// GetItems reads full metadata, including supplementary files omitted by the
// library listing endpoint. Audiobookshelf's batch/get endpoint is read-only.
func (c *Client) GetItems(ctx context.Context, ids []string) ([]Item, error) {
	data, err := json.Marshal(struct {
		IDs []string `json:"libraryItemIds"`
	}{ids})
	if err != nil {
		return nil, err
	}
	result, err := post[struct {
		Items []Item `json:"libraryItems"`
	}](ctx, c, "/items/batch/get", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// DownloadFile opens a library file download. The caller must close the body.
func (c *Client) DownloadFile(ctx context.Context, itemID, fileID string, headers http.Header) (*http.Response, error) {
	return c.getResponse(ctx, "/items/"+url.PathEscape(itemID)+"/file/"+url.PathEscape(fileID)+"/download", headers)
}

// GetCover opens a JPEG cover or thumbnail. The caller must close the body.
func (c *Client) GetCover(ctx context.Context, itemID string, thumbnail bool, headers http.Header) (*http.Response, error) {
	path := "/items/" + url.PathEscape(itemID) + "/cover?format=jpeg"
	if thumbnail {
		path += "&width=200"
	}
	return c.getResponse(ctx, path, headers)
}
