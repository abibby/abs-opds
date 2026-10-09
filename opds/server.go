package opds

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

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

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	lib, err := s.library(r.Context())
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

func (s *Server) library(ctx context.Context) (*abs.Library, error) {
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

// validateLibrary also rejects attempts to switch the configured library via URL.
func (s *Server) validateLibrary(ctx context.Context, requested string) error {
	if requested != "" && requested != s.libraryID {
		return &abs.HTTPError{StatusCode: http.StatusNotFound}
	}
	_, err := s.library(ctx)
	return err
}

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
	if err := s.validateLibrary(ctx, q.Get("library")); err != nil {
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
	var items []abs.Item
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

	for _, item := range items {
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

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	item, err := s.libraryItem(r.Context(), r.PathValue("id"))
	if err != nil {
		upstreamError(w, err)
		return
	}
	for _, file := range item.EPUBs() {
		if file.Ino != r.PathValue("file") {
			continue
		}
		filename := path.Base(strings.ReplaceAll(file.Metadata.Filename, "\\", "/"))
		if filename == "." || filename == "/" || filename == "" {
			filename = "book.epub"
		}
		if !strings.EqualFold(path.Ext(filename), ".epub") {
			filename += ".epub"
		}
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
		resp, err := s.client.DownloadFile(r.Context(), item.ID, file.Ino, r.Header)
		if err != nil {
			upstreamError(w, err)
			return
		}
		s.stream(w, r, resp, EPUBType, disposition)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) cover(w http.ResponseWriter, r *http.Request) {
	if _, err := s.libraryItem(r.Context(), r.PathValue("id")); err != nil {
		upstreamError(w, err)
		return
	}
	resp, err := s.client.GetCover(r.Context(), r.PathValue("id"), r.URL.Query().Get("thumbnail") == "1", r.Header)
	if err != nil {
		upstreamError(w, err)
		return
	}
	s.stream(w, r, resp, "image/jpeg", "")
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

func (s *Server) stream(w http.ResponseWriter, r *http.Request, resp *http.Response, contentType, disposition string) {
	defer resp.Body.Close()
	for _, name := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	w.Header().Set("Content-Type", contentType)
	if disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		if _, err := io.Copy(w, resp.Body); err != nil {
			slog.Error("Streaming Audiobookshelf response failed", "error", err)
			panic(http.ErrAbortHandler)
		}
	}
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
