package opds

import (
	"abibby.com/abs-opds/abs"
	"encoding/xml"
	"net/url"
	"time"
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
