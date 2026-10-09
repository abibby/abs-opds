package abs

import (
	"encoding/json/v2"
	"testing"
)

func TestLibraryUnmarshal(t *testing.T) {
	// Ignore metadata that the catalog does not use, including nullable settings.
	raw := []byte(`{
		"id": "main",
		"name": "Main",
		"mediaType": "book",
		"folders": [{"id": "audiobooks", "fullPath": "/audiobooks"}],
		"settings": {"autoScanCronExpression": null}
	}`)
	var lib Library
	if err := json.Unmarshal(raw, &lib); err != nil {
		t.Fatal(err)
	}
	if want := (Library{ID: "main", Name: "Main", MediaType: "book"}); lib != want {
		t.Fatalf("got %+v, want %+v", lib, want)
	}
}
