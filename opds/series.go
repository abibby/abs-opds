package opds

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

func (s *Server) series(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, limit, err := pagination(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.validateLibrary(ctx, q.Get("library")); err != nil {
		upstreamError(w, err)
		return
	}
	result, err := s.client.GetLibrarySeries(ctx, s.libraryID, page-1, limit)
	if err != nil {
		upstreamError(w, err)
		return
	}
	f := newFeed("Series", r.URL.RequestURI(), NavigationType)
	f.Links = append(f.Links, Link{Rel: "up", Type: NavigationType, Href: "/opds"})
	if !paginateFeed(&f, r, page, limit, result.Total, NavigationType) {
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}
	for _, entry := range result.Results {
		values := url.Values{"series": {entry.ID}}
		if libraryID := q.Get("library"); libraryID != "" {
			values.Set("library", libraryID)
		}
		f.Entries = append(f.Entries, Entry{
			ID: "urn:abs:series:" + entry.ID, Title: entry.Name, Updated: f.Updated,
			Links: []Link{{Rel: "subsection", Type: AcquisitionType, Href: "/opds/books?" + values.Encode()}},
		})
	}
	writeFeed(w, r, f, NavigationType)
}
