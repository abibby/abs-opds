package main

import (
	"encoding/json/v2"
	"log/slog"
	"net/http"
	"os"

	"abibby.com/abs-opds/abs"
	"github.com/joho/godotenv"
	"gosalusa.com/stream"
)

func main() {
	_ = godotenv.Load()

	c := abs.New(os.Getenv("ABS_URL"), os.Getenv("ABS_API_KEY"))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		allLibs, err := c.GetAllLibraries()
		if err != nil {
			panic(err)
		}
		bookLibs := stream.Of(allLibs.Libraries).Filter(func(l abs.Library) bool {
			return l.MediaType == "book"
		}).Slice()
		if len(bookLibs) == 0 {
			panic("no book libraries")
		}
		if len(bookLibs) > 1 {
			slog.Warn("Ignoreing libraries")
		}

		json.MarshalWrite(w, bookLibs[0])
	})
	http.ListenAndServe(":12665", nil)
}
