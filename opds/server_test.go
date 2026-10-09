package opds

import (
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/xml"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"abibby.com/abs-opds/abs"
)

func fixture(t *testing.T) http.Handler {
	t.Helper()
	file := abs.File{Ino: "42", EbookFormat: "epub", Metadata: abs.FileMetadata{Filename: "Café & book.epub", Size: 10}}
	primary := abs.Item{ID: "primary", LibraryID: "one", MediaType: "book", AddedAt: 1000, UpdatedAt: 2000,
		Media: abs.Book{EbookFile: &file, CoverPath: "/private/cover.jpg", Metadata: abs.BookMetadata{
			Title: "A & B <C>", Authors: []abs.Author{{ID: "author & jane", Name: "Jane & John"}, {ID: "shared", Name: "Shared Author"}}, Description: "A < B & C", Publisher: "Publisher", Language: "en", ISBN: "9780000000001",
			Series: []abs.Series{{ID: "shared & series", Name: "Saga & Stories", Sequence: "10"}},
		}}, LibraryFiles: []abs.File{file},
	}
	supplementary := abs.Item{ID: "supplementary", LibraryID: "one", MediaType: "book", AddedAt: 3000,
		Media: abs.Book{Metadata: abs.BookMetadata{Title: "Z book", Authors: []abs.Author{{ID: "other", Name: "Other Author"}, {ID: "shared", Name: "Shared Author"}},
			Series: []abs.Series{{ID: "shared & series", Name: "Saga & Stories", Sequence: "2"}, {ID: "other", Name: "Another series"}},
		}},
		LibraryFiles: []abs.File{{Ino: "43", Metadata: abs.FileMetadata{Filename: "extra.EPUB"}}},
	}
	audio := abs.Item{ID: "audio", LibraryID: "one", MediaType: "book"}
	audio.Media.Metadata.Authors = []abs.Author{{ID: "audio-only", Name: "Audio Author"}}
	audio.Media.Metadata.Series = []abs.Series{{ID: "audio-only", Name: "Audio only"}}
	items := map[string]abs.Item{"primary": primary, "supplementary": supplementary, "audio": audio}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Error("missing upstream bearer authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/base/api/libraries/one":
			fmt.Fprint(w, `{"id":"one","name":"Books & Audio","mediaType":"book"}`)
		case "/base/api/libraries/one/items":
			q := r.URL.Query()
			if q.Get("minified") != "1" {
				t.Error("missing minified parameter")
			}
			ids := []string{"primary", "supplementary", "audio"}
			group, encoded, _ := strings.Cut(q.Get("filter"), ".")
			value, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			switch group {
			case "authors":
				switch string(value) {
				case "author & jane":
					ids = []string{"primary"}
				case "other":
					ids = []string{"supplementary"}
				case "shared":
					ids = []string{"primary", "supplementary"}
				case "audio-only":
					ids = []string{"audio"}
				default:
					ids = nil
				}
			case "series":
				if q.Get("sort") != "sequence" && q.Get("sort") != "addedAt" {
					t.Error("series sort not sent to API")
				}
				switch string(value) {
				case "shared & series":
					ids = []string{"supplementary", "primary"}
				case "other":
					ids = []string{"supplementary"}
				case "audio-only":
					ids = []string{"audio"}
				default:
					ids = nil
				}
			case "":
			default:
				t.Errorf("unexpected filter: %s", q.Get("filter"))
			}
			if q.Get("sort") == "addedAt" {
				if q.Get("desc") != "1" {
					t.Error("recent sort must be descending")
				}
				ids = []string{"supplementary", "primary", "audio"}
			}
			page, _ := strconv.Atoi(q.Get("page"))
			limit, _ := strconv.Atoi(q.Get("limit"))
			if limit < 1 || limit > 200 {
				t.Fatalf("unbounded API request: %s", r.URL)
			}
			total := len(ids)
			start := min(page*limit, total)
			result := []abs.ItemSummary{}
			for _, id := range ids[start:min(start+limit, total)] {
				result = append(result, abs.ItemSummary{ID: id})
			}
			_ = json.MarshalWrite(w, abs.ItemsResponse{Results: result, Total: total})
		case "/base/api/libraries/one/authors":
			if r.URL.Query().Get("sort") != "name" {
				t.Error("authors must be sorted by API")
			}
			entries := []abs.Author{{ID: "author & jane", Name: "Jane & John"}, {ID: "other", Name: "Other Author"}, {ID: "shared", Name: "Shared Author"}}
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit < 1 {
				t.Fatal("unbounded author request")
			}
			start := min(page*limit, len(entries))
			_ = json.MarshalWrite(w, abs.Page[abs.Author]{Results: entries[start:min(start+limit, len(entries))], Total: len(entries)})
		case "/base/api/libraries/one/series":
			if r.URL.Query().Get("sort") != "name" {
				t.Error("series must be sorted by API")
			}
			entries := []abs.Series{{ID: "other", Name: "Another series"}, {ID: "shared & series", Name: "Saga & Stories"}}
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit < 1 {
				t.Fatal("unbounded series request")
			}
			start := min(page*limit, len(entries))
			_ = json.MarshalWrite(w, abs.Page[abs.Series]{Results: entries[start:min(start+limit, len(entries))], Total: len(entries)})
		case "/base/api/libraries/one/search":
			if r.URL.Query().Get("limit") != "50" {
				t.Error("search limit not sent")
			}
			matches := []map[string]any{}
			if r.URL.Query().Get("q") == "A & B" {
				matches = append(matches, map[string]any{"libraryItem": primary})
			}
			_ = json.MarshalWrite(w, map[string]any{"book": matches})
		case "/base/api/items/batch/get":
			if r.Method != http.MethodPost {
				t.Error("batch/get must use POST")
			}
			var req struct {
				IDs []string `json:"libraryItemIds"`
			}
			if err := json.UnmarshalRead(r.Body, &req); err != nil {
				t.Error(err)
			}
			full := []abs.Item{}
			for i := len(req.IDs) - 1; i >= 0; i-- {
				full = append(full, items[req.IDs[i]])
			}
			_ = json.MarshalWrite(w, map[string]any{"libraryItems": full})
		case "/base/api/items/primary":
			_ = json.MarshalWrite(w, primary)
		case "/base/api/items/primary/file/42/download":
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("ETag", `"v1"`)
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			if r.Header.Get("Range") == "bytes=0-3" {
				w.Header().Set("Content-Range", "bytes 0-3/10")
				w.WriteHeader(http.StatusPartialContent)
				fmt.Fprint(w, "epub")
			} else {
				fmt.Fprint(w, "epub-bytes")
			}
		case "/base/api/items/primary/cover":
			if r.URL.Query().Get("format") != "jpeg" || r.URL.Query().Get("width") != "200" {
				t.Error("unexpected cover parameters")
			}
			fmt.Fprint(w, "jpeg-bytes")
		default:
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	return New(abs.New(upstream.URL+"/base/api/", "secret-token"), "one")
}

func request(h http.Handler, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func parseFeed(t *testing.T, w *httptest.ResponseRecorder) Feed {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/atom+xml") {
		t.Fatal("not Atom XML")
	}
	if strings.Contains(w.Body.String(), "secret-token") || strings.Contains(w.Body.String(), "/private/") {
		t.Fatal("private data leaked")
	}
	var f Feed
	if err := xml.Unmarshal(w.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339, f.Updated); err != nil {
		t.Fatal(err)
	}
	return f
}

func findLink(t *testing.T, links []Link, rel string) Link {
	t.Helper()
	for _, link := range links {
		if link.Rel == rel {
			return link
		}
	}
	t.Fatalf("missing link: %s", rel)
	return Link{}
}

func TestCatalogAndAcquisition(t *testing.T) {
	h := fixture(t)
	catalog := parseFeed(t, request(h, "/opds"))
	if len(catalog.Entries) != 4 || catalog.Title != "Books & Audio" {
		t.Fatalf("catalog entries: %d", len(catalog.Entries))
	}
	search := findLink(t, catalog.Links, "search")
	if search.Href != "/opds/search?query={searchTerms}" || search.Type != "application/atom+xml" {
		t.Fatalf("search: %+v", search)
	}
	first := parseFeed(t, request(h, "/opds/books?limit=1"))
	if *first.TotalResults != 3 || len(first.Entries) != 1 || *first.StartIndex != 0 {
		t.Fatalf("bad pagination: %+v", first)
	}
	e := first.Entries[0]
	if e.Title != "A & B <C>" || e.Authors[0].Name != "Jane & John" || e.Summary.Text != "A < B & C" || e.Publisher != "Publisher" {
		t.Fatalf("metadata round trip: %+v", e)
	}
	downloads := 0
	for _, l := range e.Links {
		if l.Rel == AcquisitionRel {
			downloads++
			if l.Type != EPUBType {
				t.Error("wrong EPUB type")
			}
		}
	}
	if downloads != 1 {
		t.Fatalf("duplicate primary file: %d", downloads)
	}
	for _, rel := range []string{"cover", "thumbnail", "image", "image/thumbnail"} {
		findLink(t, e.Links, "http://opds-spec.org/"+rel)
	}
	next := findLink(t, first.Links, "next")
	second := parseFeed(t, request(h, next.Href))
	if len(second.Entries) != 1 || second.Entries[0].Title != "Z book" || *second.StartIndex != 1 {
		t.Fatalf("missing supplementary EPUB: %+v", second)
	}
	findLink(t, second.Links, "previous")
	recent := parseFeed(t, request(h, "/opds/books?sort=recent"))
	if recent.Entries[0].Title != "Z book" {
		t.Fatal("recent order incorrect")
	}
	scoped := parseFeed(t, request(h, "/opds/books?library=one"))
	if *scoped.TotalResults != 3 || scoped.Entries[0].Title != "A & B <C>" {
		t.Fatal("library filtering failed")
	}
	searched := parseFeed(t, request(h, strings.Replace(search.Href, "{searchTerms}", url.QueryEscape("A & B"), 1)))
	if len(searched.Entries) != 1 || searched.TotalResults != nil {
		t.Fatal("native search failed")
	}
	empty := parseFeed(t, request(h, "/opds/search?query=absent"))
	if empty.TotalResults != nil || len(empty.Entries) != 0 {
		t.Fatal("empty search failed")
	}
}

func TestDownloadsAndCovers(t *testing.T) {
	h := fixture(t)
	target := "/opds/items/primary/files/42/book.epub"
	w := request(h, target)
	if w.Code != 200 || w.Body.String() != "epub-bytes" || w.Header().Get("Content-Type") != EPUBType {
		t.Fatalf("download: %d %s", w.Code, w.Body.String())
	}
	_, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
	if err != nil || params["filename"] != "Café & book.epub" {
		t.Fatalf("filename: %v %v", params, err)
	}
	for _, method := range []string{"GET", "HEAD"} {
		req := httptest.NewRequest(method, target, nil)
		req.Header.Set("Range", "bytes=0-3")
		w = httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 206 || w.Header().Get("Content-Range") != "bytes 0-3/10" {
			t.Fatal("range not forwarded")
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD returned body")
		}
	}
	req := httptest.NewRequest("GET", target, nil)
	req.Header.Set("If-None-Match", `"v1"`)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatal("conditional download failed")
	}
	w = request(h, "/opds/items/primary/cover?thumbnail=1")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" || w.Body.String() != "jpeg-bytes" {
		t.Fatal("cover failed")
	}
	if request(h, "/opds/items/primary/files/not-an-epub/book.epub").Code != 404 {
		t.Fatal("arbitrary file allowed")
	}
}

func TestInvalidRequests(t *testing.T) {
	h := fixture(t)
	for _, target := range []string{"/opds/books?page=0", "/opds/books?page=-1", "/opds/books?page=NaN", "/opds/books?limit=201", "/opds/books?sort=unknown", "/opds/books?page=999999999999999999"} {
		if w := request(h, target); w.Code != 400 {
			t.Errorf("%s: %d", target, w.Code)
		}
	}
	for _, target := range []string{"/opds/books?page=999", "/opds/books?library=missing", "/unknown"} {
		if w := request(h, target); w.Code != 404 {
			t.Errorf("%s: %d", target, w.Code)
		}
	}
}

func TestUpstreamFailures(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500, 302} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(status)
				fmt.Fprint(w, "secret upstream details")
			}))
			defer upstream.Close()
			w := request(New(abs.New(upstream.URL, "key"), "one"), "/opds")
			want := 502
			if status == 403 || status == 404 {
				want = status
			}
			if w.Code != want || bytes.Contains(w.Body.Bytes(), []byte("secret")) {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
