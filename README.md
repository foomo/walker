# Walker

Walker walkes aka as crawls through websites and collects performance and SEO relevant data. The results can be browsed through a very simple web interface. Apart from that they are exposed as prometheus metrics.

**Be careful when crawling your website with walker with aggressive settings, it might take your site down**

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

### seo validation schemata

WIP

## service API

`cmd/walker` serves the gotsrpc `Service` on `/service` (`POST /service/<Method>` with a JSON array of arguments):

- `GetScrapes()` summaries of the last complete and the current scrape (either is `null` if there is none), including the performance buckets, P50, P90 and max per group
- `GetResults(filters, page, pageSize)` filtered and paged results, `filters.Scrape` selects `"current"` (default) or `"complete"`. `filters.Path` searches the paths (case insensitive, anywhere in the path, without host and query), `filters.MinDur` / `filters.MaxDur` select a duration range `[MinDur, MaxDur)` like a performance bucket, `filters.Groups` selects groups (`""` for results without one), and the filter options list all groups and performance buckets with their page counts. It also returns the folder tree of all result paths of that scrape with page and error counts, from the same snapshot as the page. A folder's `Prefix` has no trailing slash (e.g. `https://example.com/shop`); used as `filters.Prefix` it selects the folder's own page and everything below it, but not `…/shopping`. Any other prefix matches as a plain string prefix
- `GetBrokenLinks(scrape)` the broken pages (status >= 400 or failed) with the pages linking to them, the most linked first
- `GetRedirects(scrape)` the redirecting urls with their hops, where they end, whether they are permanent, and the pages linking to them - the longest chains first
- `GetLinks(scrape)` every page with the number of pages and links pointing to it and its own links; `GetLinkedFrom(scrape, url, limit)` the pages linking to one url: total, a breakdown by group, and the first `limit` pages with the most links
- `GetGraph(scrape, options)` the site as a graph of folders down to `Depth`, with edges weighted by links. Links to navigation targets - linked from more than `GlobalShare` of the pages - are left out and listed separately
- `GetPageFolderLinks(scrape, options, page)` the links of a page to the folder nodes of that graph, without navigation links and links into its own folder
- `GetFolderPages(scrape, options, folder, max)` the pages a folder node of that graph stands for, the `max` most linked, with the links between them
- `GetResult(scrape, url)` the result for a url as a browser sees it: exact match, then ignoring the fragment and credentials, then a result redirecting to the url - `null` if it was not scraped
- `GetStatus()` open / done / pending counts

The Go proxy (`gotsrpc_gen.go`) and the Swift client (`swift/`) are generated from `gotsrpc.yml`:

```sh
gotsrpc gotsrpc.yml
```

## macOS app

`macos/Walker.xcodeproj` is a native SwiftUI app (macOS 26) for one or more walker servers. It shows the current and the last complete scrape and browses the results. It compiles the generated client from `swift/` directly. The Graph section shows the site in 3D - laid out by `macos/Packages/GraphLayout` with [Grape](https://github.com/swiftgraphs/Grape)'s force simulation, rendered with RealityKit. Xcode fetches Grape when the project is opened the first time.

```sh
go run ./cmd/walker path/to/config.yaml   # serves on :3001 by default
open macos/Walker.xcodeproj                 # then run the Walker scheme
```

## metrics

Exposed on `/metrics`. Each walker has its own registry (`Walker.MetricsHandler()`), including the go and process collectors.

The same metrics are exposed for the ongoing scrape and the last completed one, distinguished by the label `scrape="current"|"complete"`. `current` is absent while walker waits for the next scheduled cycle, `complete` is absent until the first cycle has completed.

| metric | type | labels |
| --- | --- | --- |
| `walker_scrape_pages` | gauge | `scrape`, `group`, `status` |
| `walker_scrape_jobs_open` | gauge | `scrape` |
| `walker_scrape_request_duration_seconds` | histogram (report performance buckets) | `scrape`, `group` |
| `walker_scrape_started_timestamp_seconds` | gauge | `scrape` |
| `walker_scrape_completed_timestamp_seconds` | gauge | `scrape` (complete only) |
| `walker_scrape_duration_seconds` | gauge, elapsed while running | `scrape` |
| `walker_scrape_validation_score` | gauge, average per page | `scrape`, `group`, `path` |
| `walker_scrape_validation_penalty` | gauge, average per page | `scrape`, `group`, `path`, `type` |
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
| `walker_validation_score` / `_penalty` (summaries) | `walker_scrape_validation_score` / `_penalty` |
| `walker_scrape_cycle_running` | `walker_scrape_running` |
| `walker_scrape_cycle_duration_seconds` | `walker_scrape_duration_seconds{scrape="complete"}` |
| `walker_scrape_cycle_overruns_total` | `walker_scrape_overruns_total` |
| `walker_scrape_cycle_next_start_timestamp_seconds` | `walker_scrape_next_start_timestamp_seconds` |
