package abs

import (
	"encoding/json/v2"
	"testing"
)

func TestLibraryUnmarshal(t *testing.T) {
	raw := []byte(`{
		"id": "lib_5yvub9dqvctlcrza6h",
		"name": "Main",
		"folders": [
			{
				"id": "audiobooks",
				"fullPath": "/audiobooks",
				"libraryId": "main"
			}
		],
		"displayOrder": 1,
		"icon": "audiobookshelf",
		"mediaType": "book",
		"provider": "audible",
		"settings": {
			"coverAspectRatio": 1,
			"disableWatcher": false,
			"skipMatchingMediaWithAsin": false,
			"skipMatchingMediaWithIsbn": false,
			"autoScanCronExpression": null
		},
		"createdAt": 1633522963509,
		"lastUpdate": 1646520916818
	}`)

	var lib Library
	if err := json.Unmarshal(raw, &lib); err != nil {
		t.Fatalf("failed to unmarshal library: %v", err)
	}

	if lib.ID != "lib_5yvub9dqvctlcrza6h" {
		t.Errorf("expected ID lib_5yvub9dqvctlcrza6h, got %s", lib.ID)
	}
	if lib.Name != "Main" {
		t.Errorf("expected Name Main, got %s", lib.Name)
	}
	if len(lib.Folders) != 1 {
		t.Fatalf("expected 1 folder, got %d", len(lib.Folders))
	}
	folder := lib.Folders[0]
	if folder.ID != "audiobooks" || folder.FullPath != "/audiobooks" || folder.LibraryID != "main" {
		t.Errorf("unexpected folder: %+v", folder)
	}
	if lib.DisplayOrder != 1 {
		t.Errorf("expected DisplayOrder 1, got %d", lib.DisplayOrder)
	}
	if lib.Icon != "audiobookshelf" {
		t.Errorf("expected Icon audiobookshelf, got %s", lib.Icon)
	}
	if lib.MediaType != "book" {
		t.Errorf("expected MediaType book, got %s", lib.MediaType)
	}
	if lib.Provider != "audible" {
		t.Errorf("expected Provider audible, got %s", lib.Provider)
	}
	if lib.Settings.CoverAspectRatio != 1 {
		t.Errorf("expected CoverAspectRatio 1, got %d", lib.Settings.CoverAspectRatio)
	}
	if lib.Settings.DisableWatcher != false {
		t.Errorf("expected DisableWatcher false, got true")
	}
	if lib.Settings.SkipMatchingMediaWithAsin != false {
		t.Errorf("expected SkipMatchingMediaWithAsin false, got true")
	}
	if lib.Settings.SkipMatchingMediaWithIsbn != false {
		t.Errorf("expected SkipMatchingMediaWithIsbn false, got true")
	}
	if _, ok := lib.Settings.AutoScanCronExpression.Ok(); ok {
		t.Errorf("expected AutoScanCronExpression nil, got %v", lib.Settings.AutoScanCronExpression)
	}
	if lib.CreatedAt != 1633522963509 {
		t.Errorf("expected CreatedAt 1633522963509, got %d", lib.CreatedAt)
	}
	if lib.LastUpdate != 1646520916818 {
		t.Errorf("expected LastUpdate 1646520916818, got %d", lib.LastUpdate)
	}
}
