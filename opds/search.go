package opds

import (
	"context"
	"net/http"
	"strings"
	"time"

	"abibby.com/abs-opds/abs"
)

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("author") != "" || q.Get("series") != "" || q.Get("sort") != "" {
		http.Error(w, "Audiobookshelf search does not support author, series, or sort filters", http.StatusBadRequest)
		return
	}
	query := strings.TrimSpace(q.Get("query"))
	page, limit, err := pagination(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if page != 1 {
		http.Error(w, "Audiobookshelf search does not support pagination; narrow the search or increase limit", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := s.library(ctx, q.Get("library")); err != nil {
		upstreamError(w, err)
		return
	}
	var result abs.SearchResponse
	if query != "" {
		matches, err := s.client.SearchLibrary(ctx, s.libraryID, query, limit)
		if err != nil {
			upstreamError(w, err)
			return
		}
		result = *matches
	}
	f := newFeed("Search: "+query, r.URL.RequestURI(), AcquisitionType)
	f.Links = append(f.Links, Link{Rel: "up", Type: NavigationType, Href: "/opds"})
	// The API provides neither a total nor a continuation cursor for search.
	f.ItemsPerPage = limit

	for _, match := range result.Books {
		item := match.Item
		if item.LibraryID != s.libraryID || len(item.EPUBs()) == 0 {
			continue
		}
		f.Entries = append(f.Entries, bookEntry(item))
	}
	writeFeed(w, r, f, AcquisitionType)
}
