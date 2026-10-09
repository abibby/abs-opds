package opds

import (
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
)

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
		stream(w, r, resp, EPUBType, disposition)
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
	stream(w, r, resp, "image/jpeg", "")
}

func stream(w http.ResponseWriter, r *http.Request, resp *http.Response, contentType, disposition string) {
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
