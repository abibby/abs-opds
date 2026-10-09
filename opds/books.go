package opds

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"abibby.com/abs-opds/abs"
)

func (s *Server) books(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if strings.TrimSpace(q.Get("query")) != "" {
		s.search(w, r)
		return
	}
	page, limit, err := pagination(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if q.Get("sort") != "" && q.Get("sort") != "recent" {
		http.Error(w, "Invalid sort", http.StatusBadRequest)
		return
	}
	authorID, seriesID := q.Get("author"), q.Get("series")
	if authorID != "" && seriesID != "" {
		http.Error(w, "Audiobookshelf cannot combine author and series filters", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := s.library(ctx, q.Get("library")); err != nil {
		upstreamError(w, err)
		return
	}
	title, up := "All books", "/opds"
	if authorID != "" {
		title, up = "Author books", "/opds/authors"
	} else if seriesID != "" {
		title, up = "Series books", "/opds/series"
	}
	if up != "/opds" && q.Get("library") != "" {
		up += "?library=" + url.QueryEscape(q.Get("library"))
	}
	f := newFeed(title, r.URL.RequestURI(), AcquisitionType)
	f.Links = append(f.Links, Link{Rel: "up", Type: NavigationType, Href: up})
	result, err := s.client.GetLibraryItems(ctx, s.libraryID, abs.ItemQuery{
		Page:     page - 1,
		Limit:    limit,
		AuthorID: authorID,
		SeriesID: seriesID,
		Recent:   q.Get("sort") == "recent",
	})
	if err != nil {
		upstreamError(w, err)
		return
	}
	if !paginateFeed(&f, r, page, limit, result.Total, AcquisitionType) {
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}

	ids := make([]string, len(result.Results))
	for i, item := range result.Results {
		ids[i] = item.ID
	}
	items, err := s.client.GetItems(ctx, ids)
	if err != nil {
		upstreamError(w, err)
		return
	}
	// Batch responses may be unordered or contain unrelated records.
	byID := make(map[string]abs.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, id := range ids {
		item := byID[id]
		// The API cannot filter exact ebook formats (or include supplementary EPUBs
		// in an ebook-only filter). Check eligibility only within the returned page.
		if item.LibraryID != s.libraryID || len(item.EPUBs()) == 0 {
			continue
		}
		if authorID != "" {
			for _, author := range item.Media.Metadata.Authors {
				if author.ID == authorID {
					f.Title = author.Name
					break
				}
			}
		}
		if seriesID != "" {
			for _, series := range item.Media.Metadata.Series {
				if series.ID == seriesID {
					f.Title = series.Name
					break
				}
			}
		}
		f.Entries = append(f.Entries, bookEntry(item))
	}
	if q.Get("sort") == "recent" {
		f.Title = "Recently added"
	}
	writeFeed(w, r, f, AcquisitionType)
}
