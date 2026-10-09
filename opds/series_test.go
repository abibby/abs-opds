package opds

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"abibby.com/abs-opds/abs"
)

func TestSeriesNavigation(t *testing.T) {
	h := fixture(t)
	catalog := parseFeed(t, request(h, "/opds"))
	var seriesLink Link
	for _, entry := range catalog.Entries {
		if entry.Title == "Series" {
			seriesLink = findLink(t, entry.Links, "subsection")
		}
	}
	if seriesLink.Href != "/opds/series" || seriesLink.Type != NavigationType {
		t.Fatalf("bad Series navigation: %+v", seriesLink)
	}
	series := parseFeed(t, request(h, seriesLink.Href+"?limit=1"))
	if *series.TotalResults != 2 || len(series.Entries) != 1 || series.Entries[0].Title != "Another series" {
		t.Fatalf("series must be deduplicated, EPUB-only, and alphabetical: %+v", series)
	}
	next := findLink(t, series.Links, "next")
	if next.Type != NavigationType {
		t.Fatal("series pages must be navigation feeds")
	}
	series = parseFeed(t, request(h, next.Href))
	if len(series.Entries) != 1 || series.Entries[0].Title != "Saga & Stories" {
		t.Fatalf("second series page: %+v", series)
	}
	booksLink := findLink(t, series.Entries[0].Links, "subsection")
	if booksLink.Type != AcquisitionType {
		t.Fatal("series must link to acquisition feed")
	}
	books := parseFeed(t, request(h, booksLink.Href+"&limit=1"))
	if books.Title != "Saga & Stories" || *books.TotalResults != 2 || books.Entries[0].Title != "Z book" {
		t.Fatalf("series sequence 2 must precede 10: %+v", books)
	}
	if findLink(t, books.Links, "up").Href != "/opds/series" {
		t.Fatal("missing return to series")
	}
	second := parseFeed(t, request(h, findLink(t, books.Links, "next").Href))
	if second.Entries[0].Title != "A & B <C>" {
		t.Fatal("series filter lost on pagination")
	}
	scoped := parseFeed(t, request(h, "/opds/series?library=one"))
	if *scoped.TotalResults != 2 {
		t.Fatal("series library filter failed")
	}
	scopedBooks := parseFeed(t, request(h, findLink(t, scoped.Entries[0].Links, "subsection").Href))
	if *scopedBooks.TotalResults != 1 || scopedBooks.Entries[0].Title != "Z book" {
		t.Fatal("series books lost library scope")
	}
	if findLink(t, scopedBooks.Links, "up").Href != "/opds/series?library=one" {
		t.Fatal("parent lost library scope")
	}
	if w := request(h, booksLink.Href+"&query=absent"); w.Code != http.StatusBadRequest {
		t.Fatal("combined search must be rejected")
	}
	for _, target := range []string{"/opds/series?page=0", "/opds/series?limit=201"} {
		if w := request(h, target); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", target, w.Code)
		}
	}
	for _, target := range []string{"/opds/series?page=3&limit=1", "/opds/series?library=missing"} {
		if w := request(h, target); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", target, w.Code)
		}
	}
}

func TestEmptySeries(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/libraries/lib" {
			_, _ = w.Write([]byte(`{"id":"lib","mediaType":"book"}`))
		} else {
			_, _ = w.Write([]byte(`{"results":[],"total":0}`))
		}
	}))
	defer upstream.Close()
	f := parseFeed(t, request(New(abs.New(upstream.URL, "key"), "lib"), "/opds/series"))
	if *f.TotalResults != 0 || len(f.Entries) != 0 {
		t.Fatal("empty library should have an empty series feed")
	}
}
