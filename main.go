package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"abibby.com/abs-opds/abs"
	"abibby.com/abs-opds/opds"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()
	uri, key := os.Getenv("ABS_URL"), os.Getenv("ABS_API_KEY")
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("ABS_URL must be an HTTP(S) URL without credentials, query, or fragment")
	}
	if key == "" {
		return fmt.Errorf("ABS_API_KEY is required")
	}
	libraryID := strings.TrimSpace(os.Getenv("ABS_LIBRARY_ID"))
	if libraryID == "" {
		return fmt.Errorf("ABS_LIBRARY_ID is required")
	}
	handler := opds.New(abs.New(uri, key), libraryID)
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":12665"
	}
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	slog.Info("Serving OPDS", "address", addr, "path", "/opds")
	return server.ListenAndServe()
}
