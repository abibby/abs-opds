package opds

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"abibby.com/abs-opds/abs"
)

func TestLibraryIsolation(t *testing.T) {
	book := func(id, library string) abs.Item {
		return abs.Item{ID: id, LibraryID: library, MediaType: "book", Media: abs.Book{
			Metadata: abs.BookMetadata{
				Title:   id,
				Authors: []abs.Author{{ID: id, Name: id}},
				Series:  []abs.Series{{ID: id, Name: id}},
			},
			EbookFile: &abs.File{Ino: "42", EbookFormat: "epub"},
		}}
	}
	selected, foreign := book("selected-book", "one"), book("foreign-book", "two")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/libraries/one":
			_ = json.MarshalWrite(w, abs.Library{ID: "one", Name: "Selected library", MediaType: "book"})
		case "/api/libraries/one/items":
			_ = json.MarshalWrite(w, abs.ItemsResponse{Results: []abs.ItemSummary{{ID: selected.ID}}, Total: 1})
		case "/api/libraries/one/authors":
			_ = json.MarshalWrite(w, abs.Page[abs.Author]{Results: selected.Media.Metadata.Authors, Total: 1})
		case "/api/libraries/one/series":
			_ = json.MarshalWrite(w, abs.Page[abs.Series]{Results: selected.Media.Metadata.Series, Total: 1})
		case "/api/libraries/one/search":
			_ = json.MarshalWrite(w, map[string]any{"book": []map[string]any{{"libraryItem": selected}}})
		case "/api/items/batch/get":
			var body struct {
				IDs []string `json:"libraryItemIds"`
			}
			if err := json.UnmarshalRead(r.Body, &body); err != nil {
				t.Error(err)
			}
			if len(body.IDs) != 1 || body.IDs[0] != selected.ID {
				t.Errorf("unexpected batch IDs: %v", body.IDs)
			}
			// Ignore foreign records even if an upstream batch response includes one.
			_ = json.MarshalWrite(w, map[string]any{"libraryItems": []abs.Item{selected, foreign}})
		case "/api/items/foreign-book":
			_ = json.MarshalWrite(w, foreign)
		default:
			t.Errorf("unexpected upstream request outside the selected library: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	h := New(abs.New(upstream.URL, "key"), "one")
	for _, target := range []string{"/opds", "/opds/books", "/opds/books?sort=recent", "/opds/search?query=book", "/opds/series", "/opds/authors"} {
		w := request(h, target)
		feed := parseFeed(t, w)
		if strings.Contains(w.Body.String(), "foreign-book") {
			t.Fatalf("%s exposed another library", target)
		}
		if target == "/opds" {
			if feed.Title != "Selected library" || len(feed.Entries) != 4 {
				t.Fatal("catalog must contain only the selected library's navigation")
			}
		} else if target == "/opds/search?query=book" {
			if len(feed.Entries) != 1 {
				t.Fatal("missing native search result")
			}
		} else if *feed.TotalResults != 1 {
			t.Fatalf("%s: expected one result, got %d", target, *feed.TotalResults)
		}
	}
	for _, target := range []string{
		"/opds?library=two", "/opds/?library=two",
		"/opds/books?library=two", "/opds/search?library=two", "/opds/series?library=two", "/opds/authors?library=two",
		"/opds/items/foreign-book/files/42/book.epub", "/opds/items/foreign-book/cover", "/opds/items/foreign-book/cover?thumbnail=1",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, target, nil))
			if w.Code != http.StatusNotFound {
				t.Errorf("%s %s: got %d, want 404", method, target, w.Code)
			}
		}
	}
}

func TestInvalidConfiguredLibrary(t *testing.T) {
	for _, id := range []string{"", "missing", "podcasts"} {
		t.Run(id, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/libraries/missing":
					http.NotFound(w, r)
				case "/api/libraries/podcasts":
					_ = json.MarshalWrite(w, abs.Library{ID: "podcasts", MediaType: "podcast"})
				default:
					t.Errorf("invalid configuration must not fall back to another library: %s", r.URL)
				}
			}))
			defer upstream.Close()
			h := New(abs.New(upstream.URL, "key"), id)
			for _, target := range []string{"/opds", "/opds/books", "/opds/authors", "/opds/series"} {
				w := request(h, target)
				want := http.StatusNotFound
				if id == "" {
					want = http.StatusBadGateway
				}
				if w.Code != want {
					t.Errorf("%s: got %d, want %d", target, w.Code, want)
				}
			}
		})
	}
}
