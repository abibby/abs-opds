package opds

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"abibby.com/abs-opds/abs"
)

const (
	NavigationType  = "application/atom+xml;profile=opds-catalog;kind=navigation"
	AcquisitionType = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	EPUBType        = "application/epub+zip"
	AcquisitionRel  = "http://opds-spec.org/acquisition"
)

type Feed struct {
	XMLName      xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	ID           string   `xml:"id"`
	Title        string   `xml:"title"`
	Updated      string   `xml:"updated"`
	Author       Author   `xml:"author"`
	Links        []Link   `xml:"link"`
	TotalResults *int     `xml:"http://a9.com/-/spec/opensearch/1.1/ totalResults,omitempty"`
	ItemsPerPage int      `xml:"http://a9.com/-/spec/opensearch/1.1/ itemsPerPage,omitempty"`
	StartIndex   *int     `xml:"http://a9.com/-/spec/opensearch/1.1/ startIndex,omitempty"`
	Entries      []Entry  `xml:"entry"`
}

type Entry struct {
	ID         string     `xml:"id"`
	Title      string     `xml:"title"`
	Updated    string     `xml:"updated"`
	Authors    []Author   `xml:"author,omitempty"`
	Summary    *Content   `xml:"summary,omitempty"`
	Publisher  string     `xml:"http://purl.org/dc/terms/ publisher,omitempty"`
	Language   string     `xml:"http://purl.org/dc/terms/ language,omitempty"`
	Identifier string     `xml:"http://purl.org/dc/terms/ identifier,omitempty"`
	Categories []Category `xml:"category,omitempty"`
	Links      []Link     `xml:"link"`
}

type Author struct {
	Name string `xml:"name"`
}
type Content struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}
type Category struct {
	Term string `xml:"term,attr"`
}
type Link struct {
	Rel    string `xml:"rel,attr"`
	Type   string `xml:"type,attr"`
	Href   string `xml:"href,attr"`
	Title  string `xml:"title,attr,omitempty"`
	Length int64  `xml:"length,attr,omitempty"`
}

func timestamp(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339) }

func bookEntry(item abs.Item) Entry {
	m := item.Media.Metadata
	e := Entry{
		ID: "urn:abs:item:" + item.ID, Title: m.Title,
		Updated:   timestamp(max(item.UpdatedAt, item.AddedAt)),
		Publisher: m.Publisher, Language: m.Language, Identifier: m.ISBN,
	}
	if m.Description != "" {
		e.Summary = &Content{Type: "text", Text: m.Description}
	}
	for _, a := range m.Authors {
		e.Authors = append(e.Authors, Author{Name: a.Name})
	}
	if len(e.Authors) == 0 && m.AuthorName != "" {
		e.Authors = []Author{{Name: m.AuthorName}}
	}
	for _, genre := range m.Genres {
		e.Categories = append(e.Categories, Category{Term: genre})
	}
	base := "/opds/items/" + url.PathEscape(item.ID)
	for _, file := range item.EPUBs() {
		e.Links = append(e.Links, Link{
			Rel: AcquisitionRel, Type: EPUBType,
			Href:  base + "/files/" + url.PathEscape(file.Ino) + "/book.epub",
			Title: file.Metadata.Filename, Length: file.Metadata.Size,
		})
	}
	if item.Media.CoverPath != "" {
		// Include Calibre's legacy and OPDS 1.2 image relations.
		for _, rel := range []string{"cover", "thumbnail", "image", "image/thumbnail"} {
			href := base + "/cover"
			if rel == "thumbnail" || rel == "image/thumbnail" {
				href += "?thumbnail=1"
			}
			e.Links = append(e.Links, Link{Rel: "http://opds-spec.org/" + rel, Type: "image/jpeg", Href: href})
		}
	}
	return e
}

func newFeed(title, self, kind string) Feed {
	return Feed{
		ID: "urn:abs:feed:" + self, Title: title, Updated: time.Now().UTC().Format(time.RFC3339),
		Author: Author{Name: "Audiobookshelf"},
		Links: []Link{
			{Rel: "self", Type: kind, Href: self},
			{Rel: "start", Type: NavigationType, Href: "/opds"},
			// Calibre uses a direct template, which works with authenticated opds-proxy feeds.
			{Rel: "search", Type: "application/atom+xml", Href: "/opds/search?query={searchTerms}", Title: "Search"},
		},
	}
}

func pagination(q url.Values) (int, int, error) {
	page, err := positiveInt(q.Get("page"), 1)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid page")
	}
	limit, err := positiveInt(q.Get("limit"), 50)
	if err != nil || limit > 200 {
		return 0, 0, fmt.Errorf("invalid limit (1–200)")
	}
	// Avoid overflow when converting page numbers to upstream offsets.
	if page-1 > int(^uint(0)>>1)/limit {
		return 0, 0, fmt.Errorf("page is too large")
	}
	return page, limit, nil
}

// Entries have already been paginated upstream. Only build navigation links.
func paginateFeed(f *Feed, r *http.Request, page, limit, total int, kind string) bool {
	if total < 0 {
		return false
	}
	last := max(1, total/limit)
	if total%limit != 0 {
		last = total/limit + 1
	}
	if page > last {
		return false
	}
	start := (page - 1) * limit
	f.TotalResults, f.ItemsPerPage, f.StartIndex = &total, limit, &start
	link := func(rel string, page int) {
		values := r.URL.Query()
		values.Set("page", strconv.Itoa(page))
		f.Links = append(f.Links, Link{Rel: rel, Type: kind, Href: r.URL.Path + "?" + values.Encode()})
	}
	link("first", 1)
	link("last", last)
	if page > 1 {
		link("previous", page-1)
	}
	if page < last {
		link("next", page+1)
	}
	return true
}

func positiveInt(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return 0, fmt.Errorf("invalid positive integer")
	}
	return v, nil
}

func writeFeed(w http.ResponseWriter, r *http.Request, f Feed, kind string) {
	data, err := xml.MarshalIndent(f, "", "  ")
	if err != nil {
		http.Error(w, "Cannot encode feed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", kind+";charset=utf-8")
	if r.Method != http.MethodHead {
		_, _ = io.WriteString(w, xml.Header+string(data))
	}
}
