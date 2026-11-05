# Walker

Walker walkes aka as crawls through websites and collects performance and SEO relevant data. The results can be browsed through a very simple web interface. Apart from that they are exposed as prometheus metrics.

**Be careful when crawling your website with walker with aggressive settings, it might take your site down**

## Layout

A monorepo: all Go code is one module in `go/` (`github.com/foomo/walker/go`, binaries in `go/cmd`, every package in `go/internal`), next to the macOS app (`macos/`) and the editor extensions for the semantic shape language server (`editors/`).

## Development

[mise](https://mise.jdx.dev) installs Go and `gotsrpc` from `mise.toml` and runs the common tasks:

```sh
mise install
mise tasks                                   # list all tasks
mise run test
mise run walker cmd/walker/config-example.yaml   # tasks run in go/
mise run generate                            # Go proxy + Swift client
```

## Configuration

```yaml
---
# target of your scrape
target: http://www.bestbytes.de
# number of concurrent go routines
concurrency: 2
# where to run the webinterface
addr: ":3001"
# if you want to ignore <meta name="robots" content="noindex,nofollow"/>
ignorerobots: true
# in some cases using cookies is friendlier to the server
usecookies: true

# ignoring urls
## based on query parameters in this example all links, that contain a queryparameter foo
ignorequerieswith:
  - foo
## skip everything that has a query
ignoreallqueries: true
# what paths (that would be a prefixes)
ignore:
  - /foomo
# run a full scrape cycle on a wall clock aligned schedule (default 0 = restart right after completion)
# 1h => every full hour, 24h => at midnight (local time)
interval: 1h
# shift the schedule, eg interval: 24h and offset: 3h => daily at 03:00
offset: 0s
...
```

## scheduling

With an `interval`, walker idles after a complete scrape cycle until the next boundary. If a cycle does not finish within its interval, walker logs an error and still completes the cycle. It then starts the next cycle immediately, which ends on the following regular boundary, so the schedule resyncs. Overruns and the next start are exposed as prometheus metrics, see below.

## error detection

- everything greater than 400 will be tracked as an error

## external link validation (not implemented yet)

- check external links
- forbidden sites like a stage system 

## seo validation

- missing title, description, h1
- duplication title, description, h1

### semantic shapes

With `shaperoot` (and usually `groupheader`) every page is validated against the *semantic shape* of its group: an html file named `*.semanticshape.html` that describes which landmarks, headings, components, links and JSON-LD a page of that group must have, how strictly, and how it scores. The language is specified in [`go/internal/semanticshape/SPEC.md`](go/internal/semanticshape/SPEC.md), the demo shop (`mise run demo`) comes with a complete example in `go/internal/demoshop/shapes`. Writing shapes is easier with the [editor support](#editor-support).

## Editor support

`semanticshape-lsp` (`go/cmd/semanticshape-lsp`) is a language server for shape files: completion of the `val:*` elements, rules and values, hover docs, and the lint errors of the whole shape root as diagnostics while typing - unsaved buffers included. It speaks LSP over stdio and only acts on `*.semanticshape.html` files inside a shape root (a directory with `groups/` or `default.semanticshape.html`), so it runs next to the editor's own html support. The extensions in `editors/` start it from the `PATH`:

```sh
mise run lsp                      # install semanticshape-lsp into ~/go/bin
mise run editors                  # the language server + both extensions below
mise run editors:zed              # check that the Zed extension compiles (wasm32-wasip2)
mise run editors:vscode           # package editors/vscode-semanticshape/semanticshape-<version>.vsix, with the server for 5 platforms
mise run editors:vscode:install   # package and install it into VS Code
```

- **Zed** (`editors/zed-semanticshape`): `zed: install dev extension` and choose the directory - Zed builds it with your rustup toolchain. After changes: `zed: rebuild dev extension`.
- **VS Code** (`editors/vscode-semanticshape`): `mise run editors:vscode:install`. The extension bundles `semanticshape-lsp` for macOS, Linux and Windows (`editors:vscode:server` cross-compiles them into `server/`), nothing else needs to be installed. The setting `semanticshape.serverPath` points to another binary, e.g. one you are working on.

After changes to the server: for Zed run `mise run lsp` again, for VS Code `mise run editors:vscode:install` - then restart the language server (`Semantic Shape: Restart Language Server` in VS Code, `editor: restart language server` in Zed). The extensions' READMEs have the details.

## service API

`cmd/walker` serves the gotsrpc `ServiceInterface` on `/service` (`POST /service/<Method>` with a JSON array of arguments):

- `GetScrapes()` summaries of the last complete and the current scrape (either is `null` if there is none), including the performance buckets, P50, P90 and max per group
- `GetResults(filters, page, pageSize)` filtered and paged results, `filters.Scrape` selects `"current"` (default) or `"complete"`. `filters.Path` searches the paths (case insensitive, anywhere in the path, without host and query), `filters.MinDur` / `filters.MaxDur` select a duration range `[MinDur, MaxDur)` like a performance bucket, `filters.Groups` selects groups (`""` for results without one), and the filter options list all groups and performance buckets with their page counts. It also returns the folder tree of all result paths of that scrape with page and error counts, from the same snapshot as the page. A folder's `Prefix` has no trailing slash (e.g. `https://example.com/shop`); used as `filters.Prefix` it selects the folder's own page and everything below it, but not `…/shopping`. Any other prefix matches as a plain string prefix
- `GetBrokenLinks(scrape)` the broken pages (status >= 400 or failed) with the pages linking to them, the most linked first
- `GetRedirects(scrape)` the redirecting urls with their hops, where they end, whether they are permanent, and the pages linking to them - the longest chains first
- `GetLinks(scrape)` every page with the number of pages and links pointing to it and its own links; `GetLinkedFrom(scrape, url, limit)` the pages linking to one url: total, a breakdown by group, and the first `limit` pages with the most links
- `GetGraph(scrape, options)` the site as a graph of folders down to `Depth`, with edges weighted by links. Links to navigation targets - linked from more than `GlobalShare` of the pages - are left out and listed separately
- `GetPageGraph(scrape, options, page, max)` the neighborhood of a page as a graph - the page first, the pages linking to it and linked from it, the most linked up to `max`
- `GetPageFolderLinks(scrape, options, page)` the links of a page to the folder nodes of that graph, without navigation links and links into its own folder
- `GetFolderPages(scrape, options, folder, max)` the pages a folder node of that graph stands for, the `max` most linked, with the links between them
- `GetResult(scrape, url)` the result for a url as a browser sees it: exact match, then ignoring the fragment and credentials, then a result redirecting to the url - `null` if it was not scraped
- `GetStatus()` open / done / pending counts

The Go proxy (`go/internal/service/gotsrpc_gen.go`) is generated from `go/gotsrpc.yaml`, the Swift client (`macos/Walker/Generated/`) from `go/goswiftrpc.yaml`:

```sh
cd go
gotsrpc gotsrpc.yaml
goswiftrpc generate goswiftrpc.yaml
```

`mise run generate` does both; it installs `goswiftrpc` (pre-alpha, no mise support yet) with `go install github.com/foomo/goswiftrpc/cmd/goswiftrpc@latest` on every run.

## macOS app

`macos/Walker.xcodeproj` is a native SwiftUI app (macOS 26) for one or more walker servers. It shows the current and the last complete scrape and browses the results. It compiles the generated client from `macos/Walker/Generated/` directly. The app bundles the `walker` and `configvalidator` binaries, built with the Go from `mise.toml` - run `mise trust && mise install` once before building in Xcode.

```sh
go -C go run ./cmd/walker path/to/config.yaml   # serves on :3001 by default
open macos/Walker.xcodeproj                 # then run the Walker scheme
```

## metrics

Exposed on `/metrics`. Each walker has its own registry (`metrics.New()`), including the go and process collectors. The `walker_scrape_shape_*` metrics only exist with a `shaperoot`.

The same metrics are exposed for the ongoing scrape and the last completed one, distinguished by the label `scrape="current"|"complete"`. `current` is absent while walker waits for the next scheduled cycle, `complete` is absent until the first cycle has completed.

| metric | type | labels |
| --- | --- | --- |
| `walker_scrape_pages` | gauge | `scrape`, `group`, `status` |
| `walker_scrape_jobs_open` | gauge | `scrape` |
| `walker_scrape_request_duration_seconds` | histogram (report performance buckets) | `scrape`, `group` |
| `walker_scrape_started_timestamp_seconds` | gauge | `scrape` |
| `walker_scrape_completed_timestamp_seconds` | gauge | `scrape` (complete only) |
| `walker_scrape_duration_seconds` | gauge, elapsed while running | `scrape` |
| `walker_scrape_shape_score_ratio` | gauge, average share of the maximum shape score per page | `scrape`, `group`, `path` |
| `walker_scrape_shape_penalty` | gauge, average shape penalty per page | `scrape`, `group`, `path` |
| `walker_scrape_shape_findings` | gauge, average number of shape findings per page | `scrape`, `group`, `path`, `severity` |
| `walker_scrape_shape_component_score_ratio` | gauge, share of the maximum score the instances of a component reached | `scrape`, `group`, `component` |
| `walker_scrape_running` | gauge 0/1 | |
| `walker_scrape_next_start_timestamp_seconds` | gauge, only while waiting | |
| `walker_requests_total` | counter | `group`, `status` |
| `walker_scrapes_completed_total` | counter | |
| `walker_scrape_overruns_total` | counter | |

Migrating from the previous metrics:

| old | new |
| --- | --- |
| `walker_scrape_durations_seconds` (summary) | `walker_scrape_request_duration_seconds` (histogram) |
| `walker_scrape_running_total`, `walker_progress_status_code_total` | `walker_scrape_pages`, `walker_requests_total` |
| `walker_scrape_counter_total` | `sum(walker_requests_total)` |
| `walker_progress_gauge_open` / `_complete` | `walker_scrape_jobs_open` / `sum(walker_scrape_pages{scrape="current"})` |
| `walker_validation_score` / `_penalty` (summaries) | `walker_scrape_shape_score_ratio` / `walker_scrape_shape_penalty` |
| `walker_scrape_schema_score_ratio`, `_penalty`, `_findings`, `_component_score_ratio` | `walker_scrape_shape_score_ratio`, `_penalty`, `_findings`, `_component_score_ratio` |
| `walker_scrape_cycle_running` | `walker_scrape_running` |
| `walker_scrape_cycle_duration_seconds` | `walker_scrape_duration_seconds{scrape="complete"}` |
| `walker_scrape_cycle_overruns_total` | `walker_scrape_overruns_total` |
| `walker_scrape_cycle_next_start_timestamp_seconds` | `walker_scrape_next_start_timestamp_seconds` |
