package opds

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"abibby.com/abs-opds/abs"
)

// navigationPage shares pagination and link construction for authors and series.
func navigationPage[T any](s *Server, w http.ResponseWriter, r *http.Request, title, filter string,
	load func(context.Context, string, int, int) (*abs.Page[T], error),
	identify func(T) (id, name string),
) {
	q := r.URL.Query()
	page, limit, err := pagination(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := s.library(ctx, q.Get("library")); err != nil {
		upstreamError(w, err)
		return
	}
	result, err := load(ctx, s.libraryID, page-1, limit)
	if err != nil {
		upstreamError(w, err)
		return
	}
	f := newFeed(title, r.URL.RequestURI(), NavigationType)
	f.Links = append(f.Links, Link{Rel: "up", Type: NavigationType, Href: "/opds"})
	if !paginateFeed(&f, r, page, limit, result.Total, NavigationType) {
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}
	for _, entry := range result.Results {
		id, name := identify(entry)
		values := url.Values{filter: {id}}
		if libraryID := q.Get("library"); libraryID != "" {
			values.Set("library", libraryID)
		}
		f.Entries = append(f.Entries, Entry{
			ID: "urn:abs:" + filter + ":" + id, Title: name, Updated: f.Updated,
			Links: []Link{{Rel: "subsection", Type: AcquisitionType, Href: "/opds/books?" + values.Encode()}},
		})
	}
	writeFeed(w, r, f, NavigationType)
}
