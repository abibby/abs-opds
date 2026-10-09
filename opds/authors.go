package opds

import (
	"net/http"

	"abibby.com/abs-opds/abs"
)

func (s *Server) authors(w http.ResponseWriter, r *http.Request) {
	navigationPage(s, w, r, "Authors", "author", s.client.GetLibraryAuthors, func(entry abs.Author) (string, string) {
		return entry.ID, entry.Name
	})
}
