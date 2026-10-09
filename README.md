# abs-opds

Expose EPUBs from Audiobookshelf as an OPDS 1.x Atom catalog for
[opds-proxy](https://github.com/evan-buss/opds-proxy) and other OPDS readers.

The feed follows the conventions used by Calibre: navigation and acquisition
feeds, direct search templates, EPUB acquisition links, and both legacy and
OPDS 1.2 cover/thumbnail links. Downloads are streamed through abs-opds with
an EPUB MIME type and attachment filename so opds-proxy can convert them.
Audiobookshelf credentials stay on the server.

## Run

Use the Go version specified in `go.mod` (Go 1.27). Create a `.env` using
`.env.example` as a template, or supply these environment variables:

```dotenv
ABS_URL=http://localhost:13378
ABS_API_KEY=your-audiobookshelf-api-key
ABS_LIBRARY_ID=your-audiobookshelf-library-id
LISTEN_ADDR=:12665
```

`ABS_URL` accepts the server URL, a subpath, or a URL ending in `/api`.
`ABS_LIBRARY_ID` is required and selects the only library exposed by this server.
Use an Audiobookshelf API key with access to that book library and permission
to download its files. Environment variables take precedence over `.env`.

```sh
go run .
```

The catalog is at `http://localhost:12665/opds`. The root URL redirects there.
The catalog uses the selected library's name and includes only its books.
Primary and supplementary EPUB files are exposed; audio-only items, PDFs,
podcasts, and missing or invalid items are excluded.

## Docker

```sh
docker build -t abs-opds .
docker run --rm --name abs-opds --env-file .env -p 12665:12665 abs-opds
```

Set `ABS_URL` in `.env` to an address reachable from the container; `localhost`
inside the container refers to the container itself. For Audiobookshelf running
on the same Docker network, use its service name, such as
`http://audiobookshelf:13378`, and add `--network NETWORK_NAME` to `docker run`.

The image runs as a non-root user and includes CA certificates for HTTPS
connections. Credentials are passed at runtime and excluded from the build context.

## Docker Compose

The included `compose.yaml` builds abs-opds and starts opds-proxy with the
Audiobookshelf feed already configured. Set `ABS_URL`, `ABS_API_KEY`, and `ABS_LIBRARY_ID` in `.env`
to connect to your existing Audiobookshelf server, then run:

```sh
docker compose up -d --build
```

Open `http://localhost:8080` for the opds-proxy interface. Set `OPDS_PROXY_PORT`
in `.env` to change the host port. Only opds-proxy publishes a host port; it
reaches abs-opds at `http://abs-opds:12665/opds` on the Compose network.
Compose fixes the internal OPDS listening port at 12665.

`ABS_URL` must be reachable from the Compose network. Audiobookshelf is not
started by this file; use its reachable hostname or LAN address, or connect
abs-opds to its existing Docker network.

If `OPDS_USERNAME` and `OPDS_PASSWORD` are set, Compose supplies them to both
services automatically. These authenticate the connection between opds-proxy
and abs-opds; they do not protect the opds-proxy web interface.

```sh
docker compose down
```

## Connect opds-proxy

Add this to opds-proxy's `config.yml`, using a hostname reachable from its process
or container:

```yaml
feeds:
  - name: Audiobookshelf
    url: http://abs-opds:12665/opds
```

For optional HTTP Basic authentication, set both `OPDS_USERNAME` and
`OPDS_PASSWORD` in abs-opds and add matching credentials to the proxy feed:

```yaml
feeds:
  - name: Audiobookshelf
    url: http://abs-opds:12665/opds
    auth:
      username: reader
      password: change-me
```

Without these settings, everyone able to reach abs-opds can browse and download
books available to its API key. Use a private network or authenticated HTTPS
reverse proxy as appropriate. Mount abs-opds at the host root; feed links use
root-relative `/opds/...` paths.

## Catalog

- `/opds`: all books, recently added, series, and authors from the configured library.
- `/opds/authors`: author navigation sorted by Audiobookshelf.
- `/opds/books?author=AUTHOR_ID`: an author's EPUB books, sorted by title.
- `/opds/series`: series navigation sorted by Audiobookshelf.
- `/opds/books?series=SERIES_ID`: books filtered and ordered by series position
  in Audiobookshelf.
- `/opds/books`: EPUB books sorted by title.
- `/opds/books?sort=recent`: newest additions first.
- `/opds/search?query=WORDS`: Audiobookshelf's native book search (titles,
  subtitles, ISBNs, and ASINs).

Author, series, and book feeds accept `page` (one-based) and `limit` (default 50,
maximum 200). Filters, sorting, and pagination run in Audiobookshelf. Each request
retrieves one page; only that page's books are expanded to discover EPUB files.
Pagination totals describe upstream items, not EPUB-only counts. Pages may be
short or empty when upstream results contain non-EPUB items; next/previous links
still navigate the upstream pages. Authors and series without EPUBs may appear.

Audiobookshelf accepts only one category filter, so combining `author`, `series`,
or `query` returns 400. Native search accepts `limit` but has no pagination,
sorting, or total count; narrow the search if its limited results are insufficient.
Search responses omit total counts and page navigation. Downloads support byte
ranges and conditional requests when Audiobookshelf supports them.

The optional `library` query parameter can only match `ABS_LIBRARY_ID`; other
values return 404. Direct downloads and covers from other libraries also return
404, even if the configured API key has access to them. An unavailable or non-book
library returns an error; the server never falls back to another library.

Book feeds retrieve page metadata through Audiobookshelf's read-only
`/api/items/batch/get` endpoint, preserving the API's result order. Search returns
expanded metadata directly. Exact EPUB eligibility and library access are checked
only on returned records, since the API's ebook filter excludes supplementary
files and cannot select just EPUBs. No request scans the whole library.
Catalog collection has a 30-second deadline; streamed downloads have a five-minute
timeout. This implements the feed conventions needed by opds-proxy, rather than
all Calibre catalog categories or its search query language.

## Development

```sh
go test -race ./...
go vet ./...
```

Tests use a local mock Audiobookshelf server, including minified listing results,
library isolation, supplementary EPUBs, XML escaping, pagination, search,
download headers/ranges, and upstream failures.

Compatibility references:
[opds-proxy's parser](https://github.com/evan-buss/opds-proxy/tree/main/opds),
[Calibre's feed implementation](https://github.com/kovidgoyal/calibre/blob/master/src/calibre/srv/opds.py),
and [Audiobookshelf's item controller](https://github.com/advplyr/audiobookshelf/blob/master/server/controllers/LibraryItemController.js).
