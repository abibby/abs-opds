package opds

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"abibby.com/abs-opds/abs"
)

type Server struct {
	client    *abs.Client
	libraryID string
}

func New(client *abs.Client, libraryID string) http.Handler {
	s := &Server{client: client, libraryID: strings.TrimSpace(libraryID)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/opds", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("GET /opds", s.catalog)
	mux.HandleFunc("GET /opds/{$}", s.catalog)
	mux.HandleFunc("GET /opds/books", s.books)
	mux.HandleFunc("GET /opds/series", s.series)
	mux.HandleFunc("GET /opds/authors", s.authors)
	mux.HandleFunc("GET /opds/search", s.search)
	mux.HandleFunc("GET /opds/items/{id}/files/{file}/book.epub", s.download)
	mux.HandleFunc("GET /opds/items/{id}/cover", s.cover)
	return mux
}

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	lib, err := s.library(r.Context(), r.URL.Query().Get("library"))
	if err != nil {
		upstreamError(w, err)
		return
	}
	f := newFeed(lib.Name, "/opds", NavigationType)
	add := func(id, title, href string, updated int64) {
		f.Entries = append(f.Entries, Entry{ID: "urn:abs:catalog:" + id, Title: title, Updated: timestamp(updated),
			Links: []Link{{Rel: "subsection", Type: AcquisitionType, Href: href}},
		})
	}
	add("all", "All books", "/opds/books", 0)
	add("recent", "Recently added", "/opds/books?sort=recent", 0)
	f.Entries = append(f.Entries, Entry{
		ID: "urn:abs:catalog:series", Title: "Series", Updated: f.Updated,
		Links: []Link{{Rel: "subsection", Type: NavigationType, Href: "/opds/series"}},
	}, Entry{
		ID: "urn:abs:catalog:authors", Title: "Authors", Updated: f.Updated,
		Links: []Link{{Rel: "subsection", Type: NavigationType, Href: "/opds/authors"}},
	})
	writeFeed(w, r, f, NavigationType)
}

func (s *Server) library(ctx context.Context, requested string) (*abs.Library, error) {
	if requested != "" && requested != s.libraryID {
		return nil, &abs.HTTPError{StatusCode: http.StatusNotFound}
	}
	if s.libraryID == "" {
		return nil, fmt.Errorf("ABS_LIBRARY_ID is required")
	}
	lib, err := s.client.GetLibrary(ctx, s.libraryID)
	if err != nil {
		return nil, err
	}
	if lib.ID != s.libraryID || lib.MediaType != "book" {
		return nil, &abs.HTTPError{StatusCode: http.StatusNotFound}
	}
	return lib, nil
}

func (s *Server) libraryItem(ctx context.Context, id string) (*abs.Item, error) {
	if s.libraryID == "" {
		return nil, fmt.Errorf("ABS_LIBRARY_ID is required")
	}
	item, err := s.client.GetItem(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.LibraryID != s.libraryID || len(item.EPUBs()) == 0 {
		return nil, &abs.HTTPError{StatusCode: http.StatusNotFound}
	}
	return item, nil
}

func upstreamError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var upstream *abs.HTTPError
	if errors.As(err, &upstream) {
		switch upstream.StatusCode {
		case http.StatusNotFound, http.StatusForbidden, http.StatusRequestedRangeNotSatisfiable:
			status = upstream.StatusCode
		}
	}
	slog.Error("Audiobookshelf request failed", "error", err)
	http.Error(w, http.StatusText(status), status)
}
