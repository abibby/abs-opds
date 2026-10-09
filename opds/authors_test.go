package opds

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"abibby.com/abs-opds/abs"
)

func TestAuthorsNavigation(t *testing.T) {
	h := fixture(t)
	catalog := parseFeed(t, request(h, "/opds"))
	var authorsLink Link
	for _, entry := range catalog.Entries {
		if entry.Title == "Authors" {
			authorsLink = findLink(t, entry.Links, "subsection")
		}
	}
	if authorsLink.Href != "/opds/authors" || authorsLink.Type != NavigationType {
		t.Fatalf("bad Authors navigation: %+v", authorsLink)
	}
	first := parseFeed(t, request(h, authorsLink.Href+"?limit=2"))
	if *first.TotalResults != 3 || len(first.Entries) != 2 || first.Entries[0].Title != "Jane & John" || first.Entries[1].Title != "Other Author" {
		t.Fatalf("authors must preserve the upstream page and order: %+v", first)
	}
	booksLink := findLink(t, first.Entries[0].Links, "subsection")
	if booksLink.Type != AcquisitionType {
		t.Fatal("author must link to an acquisition feed")
	}
	books := parseFeed(t, request(h, booksLink.Href))
	if books.Title != "Jane & John" || *books.TotalResults != 1 || books.Entries[0].Title != "A & B <C>" {
		t.Fatalf("author filtering or escaping failed: %+v", books)
	}
	if findLink(t, books.Links, "up").Href != "/opds/authors" {
		t.Fatal("missing return to Authors")
	}
	next := findLink(t, first.Links, "next")
	if next.Type != NavigationType {
		t.Fatal("author pagination must use navigation links")
	}
	second := parseFeed(t, request(h, next.Href))
	if len(second.Entries) != 1 || second.Entries[0].Title != "Shared Author" {
		t.Fatalf("coauthor missing from next page: %+v", second)
	}
	sharedLink := findLink(t, second.Entries[0].Links, "subsection")
	shared := parseFeed(t, request(h, sharedLink.Href+"&limit=1"))
	if *shared.TotalResults != 2 || shared.Entries[0].Title != "A & B <C>" {
		t.Fatal("coauthored books must be included and ordered by title")
	}
	lastBook := parseFeed(t, request(h, findLink(t, shared.Links, "next").Href))
	if lastBook.Entries[0].Title != "Z book" {
		t.Fatal("author filter lost during pagination")
	}
	for _, entry := range lastBook.Entries {
		if findLink(t, entry.Links, AcquisitionRel).Type != EPUBType {
			t.Fatal("missing EPUB acquisition")
		}
	}
	scoped := parseFeed(t, request(h, "/opds/authors?library=one"))
	if *scoped.TotalResults != 3 {
		t.Fatal("author library filter failed")
	}
	scopedBooks := parseFeed(t, request(h, findLink(t, scoped.Entries[1].Links, "subsection").Href))
	if *scopedBooks.TotalResults != 1 || findLink(t, scopedBooks.Links, "up").Href != "/opds/authors?library=one" {
		t.Fatal("library scope lost in author books or parent link")
	}
	for _, target := range []string{sharedLink.Href + "&query=absent", sharedLink.Href + "&series=other"} {
		if w := request(h, target); w.Code != http.StatusBadRequest {
			t.Fatal("unsupported combined filters must be rejected")
		}
	}
	for _, target := range []string{"/opds/authors?page=0", "/opds/authors?limit=201", "/opds/authors?limit=invalid"} {
		if w := request(h, target); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", target, w.Code)
		}
	}
	for _, target := range []string{"/opds/authors?page=4&limit=1", "/opds/authors?library=missing"} {
		if w := request(h, target); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", target, w.Code)
		}
	}
}

func TestAuthorsEmptyAndUpstreamFailure(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if r.URL.Path == "/api/libraries/lib" {
					_, _ = w.Write([]byte(`{"id":"lib","mediaType":"book"}`))
				} else {
					_, _ = w.Write([]byte(`{"results":[],"total":0}`))
				}
			}))
			defer upstream.Close()
			w := request(New(abs.New(upstream.URL, "key"), "lib"), "/opds/authors")
			if status != http.StatusOK {
				if w.Code != http.StatusBadGateway {
					t.Fatalf("upstream error: %d", w.Code)
				}
				return
			}
			f := parseFeed(t, w)
			if *f.TotalResults != 0 || len(f.Entries) != 0 {
				t.Fatal("expected empty authors feed")
			}
		})
	}
}
