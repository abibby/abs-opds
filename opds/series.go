package opds

import (
	"net/http"

	"abibby.com/abs-opds/abs"
)

func (s *Server) series(w http.ResponseWriter, r *http.Request) {
	navigationPage(s, w, r, "Series", "series", s.client.GetLibrarySeries, func(entry abs.Series) (string, string) {
		return entry.ID, entry.Name
	})
}
