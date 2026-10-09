package opds

import (
	"encoding/base64"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"abibby.com/abs-opds/abs"
)

func TestAPIPageRequests(t *testing.T) {
	encode := func(id string) string { return base64.StdEncoding.EncodeToString([]byte(id)) }
	for _, tc := range []struct {
		name, target, endpoint string
		query                  url.Values
		books                  bool
	}{
		{"all", "/opds/books?page=3&limit=2", "items", url.Values{"page": {"2"}, "limit": {"2"}, "sort": {"media.metadata.title"}, "minified": {"1"}}, true},
		{"author", "/opds/books?author=a%26b&page=3&limit=2", "items", url.Values{"page": {"2"}, "limit": {"2"}, "sort": {"media.metadata.title"}, "minified": {"1"}, "filter": {"authors." + encode("a&b")}}, true},
		{"series", "/opds/books?series=s%2F1&page=3&limit=2", "items", url.Values{"page": {"2"}, "limit": {"2"}, "sort": {"sequence"}, "minified": {"1"}, "filter": {"series." + encode("s/1")}}, true},
		{"recent", "/opds/books?sort=recent&page=3&limit=2", "items", url.Values{"page": {"2"}, "limit": {"2"}, "sort": {"addedAt"}, "minified": {"1"}, "desc": {"1"}}, true},
		{"authors", "/opds/authors?page=3&limit=2", "authors", url.Values{"page": {"2"}, "limit": {"2"}, "sort": {"name"}}, false},
		{"series list", "/opds/series?page=3&limit=2", "series", url.Values{"page": {"2"}, "limit": {"2"}, "sort": {"name"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pageCalls, batchCalls := 0, 0
			makeBook := func(id string) abs.Item {
				return abs.Item{ID: id, LibraryID: "lib", MediaType: "book", Media: abs.Book{Metadata: abs.BookMetadata{Title: id}, EbookFile: &abs.File{Ino: id, EbookFormat: "epub"}}}
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/libraries/lib":
					_ = json.MarshalWrite(w, abs.Library{ID: "lib", MediaType: "book"})
				case "/api/libraries/lib/" + tc.endpoint:
					pageCalls++
					if !reflect.DeepEqual(r.URL.Query(), tc.query) {
						t.Errorf("API query: got %v, want %v", r.URL.Query(), tc.query)
					}
					if tc.books {
						// Deliberately not alphabetical: the server must preserve API order.
						_, _ = w.Write([]byte(`{"results":[{"id":"z","media":{"metadata":{"series":{"id":"s/1","name":"Saga","sequence":"1"}}}},{"id":"a"}],"total":10000}`))
					} else {
						_ = json.MarshalWrite(w, abs.Page[abs.Author]{Results: []abs.Author{{ID: "z", Name: "Zed"}, {ID: "a", Name: "Alpha"}}, Total: 10000})
					}
				case "/api/items/batch/get":
					batchCalls++
					var body struct {
						IDs []string `json:"libraryItemIds"`
					}
					if err := json.UnmarshalRead(r.Body, &body); err != nil {
						t.Error(err)
					}
					if !reflect.DeepEqual(body.IDs, []string{"z", "a"}) {
						t.Errorf("batch fetched more than the requested page: %v", body.IDs)
					}
					_ = json.MarshalWrite(w, map[string]any{"libraryItems": []abs.Item{makeBook("a"), makeBook("z"), makeBook("extra")}})
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()
			feed := parseFeed(t, request(New(abs.New(upstream.URL, "key"), "lib"), tc.target))
			if pageCalls != 1 || (tc.books && batchCalls != 1) || (!tc.books && batchCalls != 0) {
				t.Fatalf("unexpected scanning: %d page calls, %d batch calls", pageCalls, batchCalls)
			}
			if len(feed.Entries) != 2 || *feed.TotalResults != 10000 || *feed.StartIndex != 4 {
				t.Fatalf("upstream pagination lost: %+v", feed)
			}
			wantTitle := "Zed"
			if tc.books {
				wantTitle = "z"
			}
			if feed.Entries[0].Title != wantTitle {
				t.Fatal("API order changed")
			}
			next, _ := url.Parse(findLink(t, feed.Links, "next").Href)
			original, _ := url.Parse(tc.target)
			expected := original.Query()
			expected.Set("page", "4")
			if !reflect.DeepEqual(next.Query(), expected) {
				t.Fatal("pagination lost filters")
			}
		})
	}
}

func TestPageWithoutEPUBs(t *testing.T) {
	h := fixture(t)
	f := parseFeed(t, request(h, "/opds/books?page=3&limit=1"))
	if len(f.Entries) != 0 || *f.TotalResults != 3 {
		t.Fatal("non-EPUB page must retain upstream pagination")
	}
	findLink(t, f.Links, "previous")
	for _, target := range []string{"/opds/books?author=missing", "/opds/books?series=missing"} {
		f := parseFeed(t, request(h, target))
		if *f.TotalResults != 0 || len(f.Entries) != 0 {
			t.Fatal("missing API matches should produce empty feed")
		}
	}
}

func TestUnsupportedAPIFilters(t *testing.T) {
	// Unsupported combinations must not silently drop a filter or scan the library.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Errorf("unexpected upstream request: %s", r.URL) }))
	defer upstream.Close()
	h := New(abs.New(upstream.URL, "key"), "lib")
	for _, target := range []string{
		"/opds/books?author=a&series=s", "/opds/search?query=q&author=a", "/opds/search?query=q&series=s", "/opds/search?query=q&sort=recent", "/opds/search?query=q&page=2",
		"/opds/books?query=q&author=a", "/opds/books?query=q&series=s", "/opds/books?query=q&sort=recent", "/opds/books?query=q&page=2",
	} {
		if w := request(h, target); w.Code != 400 {
			t.Errorf("%s returned %d", target, w.Code)
		}
	}
}

func TestBookPageBatchResults(t *testing.T) {
	for _, tc := range []struct {
		name       string
		ids        []string
		batchError bool
		wantStatus int
		wantBooks  int
	}{
		{name: "empty page", wantStatus: http.StatusOK},
		{name: "missing and foreign records", ids: []string{"missing", "foreign", "book"}, wantStatus: http.StatusOK, wantBooks: 1},
		{name: "batch failure", ids: []string{"book"}, batchError: true, wantStatus: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			batchCalls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/libraries/lib":
					_ = json.MarshalWrite(w, abs.Library{ID: "lib", MediaType: "book"})
				case "/api/libraries/lib/items":
					page := abs.ItemsResponse{Total: len(tc.ids)}
					for _, id := range tc.ids {
						page.Results = append(page.Results, abs.ItemSummary{ID: id})
					}
					_ = json.MarshalWrite(w, page)
				case "/api/items/batch/get":
					batchCalls++
					if tc.batchError {
						http.Error(w, "private upstream details", http.StatusInternalServerError)
						return
					}
					book := abs.Item{ID: "book", LibraryID: "lib", MediaType: "book", Media: abs.Book{EbookFile: &abs.File{Ino: "42", EbookFormat: "epub"}}}
					foreign := book
					foreign.ID, foreign.LibraryID = "foreign", "other-library"
					_ = json.MarshalWrite(w, map[string]any{"libraryItems": []abs.Item{book, foreign}})
				default:
					t.Errorf("unexpected upstream request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()
			w := request(New(abs.New(upstream.URL, "key"), "lib"), "/opds/books")
			if w.Code != tc.wantStatus {
				t.Fatalf("got HTTP %d, want %d: %s", w.Code, tc.wantStatus, w.Body.String())
			}
			if want := min(1, len(tc.ids)); batchCalls != want {
				t.Fatalf("got %d batch calls, want %d", batchCalls, want)
			}
			if tc.wantStatus == http.StatusOK {
				feed := parseFeed(t, w)
				if len(feed.Entries) != tc.wantBooks || *feed.TotalResults != len(tc.ids) {
					t.Fatalf("unexpected book page: %+v", feed)
				}
				if tc.wantBooks > 0 && feed.Entries[0].ID != "urn:abs:item:book" {
					t.Fatalf("unexpected book: %+v", feed.Entries[0])
				}
			}
		})
	}
}
