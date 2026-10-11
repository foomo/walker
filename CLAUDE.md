# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Walker crawls a website and collects performance, SEO and broken-link data. It serves the results through a minimal web UI (`/`, `/status`, `/reports/...`) and as Prometheus metrics (`/metrics`). Aggressive settings (high `concurrency`) can take a target site down, so keep that in mind when you run it against real hosts.

## Commands

```sh
go build ./...                                   # build everything
go run ./cmd/walker path/to/config.yaml          # run the crawler + web UI (default addr :3001)
go run ./cmd/validator schema.html https://host/page   # validate one page against an htmlschema file
go test ./...                                    # all tests
go test ./config -run TestLoad                   # single test
go test ./htmlschema/... -v                      # schema loader/group validator + example server
```

The `Makefile` holds only ad-hoc deploy targets (cross-compile, then scp to a stage host); it isn't a build system. The `Dockerfile` builds both binaries and bakes in `cmd/walker/config-example.yaml`.

## Configuration

`config.Load` parses the YAML into an internal `config` struct and then normalizes it into the public `config.Config`:
- `target` can be a plain URL string or a map `{baseurl, paths: [...]}`. If `paths` is missing, the path in the URL becomes the single start path and `BaseURL` keeps only the scheme and host.
- Defaults: `concurrency: 2`, `addr: ":3001"`, `usecookies: true`, `agent: foomo-walker`.
- `interval` / `offset` (durations): wall-clock-aligned scrape schedule, see `schedule.go`. `0` means restart immediately.
- Other keys: `ignore` (path prefixes), `ignorequerieswith`, `ignoreallqueries`, `ignorerobots`, `depth`, `paging`, `groupheader`, `schemaroot`.

Most `cmd/walker/*.yaml` files are local, site-specific configs and are usually untracked.

## Architecture

**Actor-style core (`walker.go`, `scrapeloop.go`).** `NewWalker` starts one goroutine, `scrapeloop`, which owns all crawl state: the job map, results and the client pool. Public methods (`Walk`, `Stop`, `GetStatus`, `GetStatuses`) never touch that state directly. Each one sends a request on a channel and waits for the reply on that same channel. Keep it this way: new features that read or change crawl state should add a channel case to the `select` in `scrapeloop`, not shared fields or mutexes. `GetStatuses()` returns the last completed cycle and the running one (`nil` while idle between scheduled cycles), and the reports render both. `GetStatus()` returns the current cycle.

**Scraping (`scrape.go`).** `scrapeloop` hands each job a free `poolClient` from `clientPool` (one `http.Client` per unit of concurrency, optional cookie jar, TLS verification disabled, redirect chain recorded via request context) and calls `scrape` in its own goroutine. Results come back over `chanResult`. `scrape` parses the HTML with goquery and runs `extract.go`, which pulls out title, description, h1, canonical and JSON-LD into `vo.Structure`. It also collects links (including links inside `<noscript>`) and, if configured, runs schema validation.

**Extension hooks.** `Walk`/`NewService` take four optional funcs (`cmd/walker` passes `nil` for all of them), so library users can customize crawling:
- `LinkListFilterFunc`: which links to follow. The default lives in `filterscrapelinks.go`; it normalizes links, stays on the host, and applies the ignore/query/depth/paging rules plus robots.txt (`robotstxt.go`).
- `ScrapeFunc`: extracts custom data from the response.
- `ValidationFunc`: produces `vo.Validations` from the structure and the scrape data.
- `ScrapeResultModifierFunc`: post-processes each result.

**htmlschema.** Validation schemas are HTML documents annotated with `val:*` attributes (e.g. `val:score`, `val:attr="content;min-length:10"`), `content=*` wildcards and `<ref>` includes. `NewGroupValidator(schemaroot)` loads `<root>/<group>/<name>.html` plus an optional `<root>/default.html`. A page's group comes from the response header named by `groupheader`. `htmlschema/example/` contains a working schema tree and a test server.

**Reports (`reports/`).** Each report renders plain text/HTML from a `vo.Status`, and `reports.GetReportHandler(basePath)` routes between them. To add a report, register it in `reports.go`, both in the handler and in `GetReportHandlerMenuHTML`.

**Service API and macOS app.** `service.go` is exposed via gotsrpc: `gotsrpc.yml` generates the Go HTTP proxy (`gotsrpc_gen.go`, mounted on `/service` in `cmd/walker`) and a Swift client in `swift/`. Regenerate with `gotsrpc gotsrpc.yml` after changing `Service` or any type it returns. Never edit `*_gen.go` or `swift/*.swift` by hand. `macos/Walker.xcodeproj` is a SwiftUI app (macOS 26, Swift 6) that compiles `swift/` and `macos/Walker/` into one module through synchronized folders, so new files need no project edits. Constraints that come from the generated Swift code:
- All mapped Go packages share one Swift module, so Go type names must be unique across `vo`, `htmlschema` and `walker` (hence `vo.ResultValidation`).
- A zero `time.Time` is encoded as `null`, but a value `time.Time` becomes a non-optional `Date`. Use `*time.Time` for timestamps that can be unset (see `vo.ScrapeSummary`).
- The generated `Duration` (go `time.Duration`, nanoseconds) shadows `Swift.Duration` in the app, so write `Swift.Duration` explicitly.
- The 3D graph (`Views/GraphView.swift`, RealityKit) is laid out by the local package `macos/Packages/GraphLayout`, the only user of Grape's `ForceSimulation` (pinned to a commit on `main` - `kinetics.position` is only public there). Grape is generic and `@inlinable`, so its hot loops compile in the calling module; the package always builds with `-O` via `unsafeFlags` (not `-wmo`, that breaks Xcode debug builds), or the layout is ~50x slower in Debug builds. Keep simulation code in that package. Xcode resolves Grape on first open.
- The app target must not use `MainActor` default isolation (it would isolate the generated `Codable` conformances). App model classes are `@MainActor` explicitly.

**Other pieces.** `service.go` wraps the Walker with filtering and paging over results. `metrics.go` gives each Walker its own Prometheus registry (`MetricsHandler()`). The loop only updates the few true counters. All per-scrape metrics come from a collector that derives them from `GetStatuses()` at scrape time, once per view, with `scrape="current"|"complete"`. Add new per-scrape metrics there, not as state in `scrapeloop`. `validationmetrics.go` aggregates schema validation reports for it. `vo/` holds the shared value objects (`Status`, `ScrapeResult`, `Structure`, `Validations`).
