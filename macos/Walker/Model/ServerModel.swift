import Foundation
import Observation

/// ServerModel is the state of one walker server: its scrapes and a page of results.
///
/// It is the connection boundary of the app - a walker bundled with the app
/// will be just another server on localhost.
@MainActor
@Observable
final class ServerModel {
    enum Connection: Equatable {
        case unknown
        case online
        case offline(String)
    }

    /// Scrape mirrors the scrape label of the walker metrics and Filters.scrape.
    enum Scrape: String, CaseIterable, Identifiable {
        case current
        case complete

        var id: Self { self }

        var title: String {
            switch self {
            case .current: "Current"
            case .complete: "Last complete"
            }
        }
    }

    /// DurationRange is a performance bucket as a filter - [from, to) in nanoseconds
    struct DurationRange: Hashable {
        let from: Int64
        let to: Int64

        init(_ bucket: PerformanceBucket) {
            from = bucket.from.rawValue
            to = bucket.to.rawValue
        }
    }

    struct ResultsQuery: Hashable {
        var scrape: Scrape
        var prefix: String
        var search: String
        var status: Int?
        var group: String?
        var duration: DurationRange?
        var page: Int
    }

    let server: Server
    @ObservationIgnored private let client: ServiceClient

    private(set) var connection: Connection = .unknown
    private(set) var current: ScrapeSummary?
    private(set) var complete: ScrapeSummary?

    // results query - changing a filter starts over at the first page
    var scrape: Scrape = .current {
        didSet { if scrape != oldValue { page = 0 } }
    }
    var prefix = "" {
        didSet { if prefix != oldValue { page = 0 } }
    }
    /// search filters by the path of the results - see Filters.path
    var search = "" {
        didSet { if search != oldValue { page = 0 } }
    }
    var status: Int? {
        didSet { if status != oldValue { page = 0 } }
    }
    /// group filters by group - "" are the results without a group
    var group: String? {
        didSet { if group != oldValue { page = 0 } }
    }
    /// duration filters by a performance bucket
    var duration: DurationRange? {
        didSet { if duration != oldValue { page = 0 } }
    }
    var page = 0
    let pageSize = 100

    private(set) var results: [ScrapeResult] = []
    /// resultsPrefix is the prefix the results were loaded with - render them
    /// relative to it, not to the prefix being edited
    private(set) var resultsPrefix = ""
    /// folderPage is the page of the folder resultsPrefix itself, e.g. the
    /// result for https://example.com/shop or .../shop/ - it sorts first, so it is taken
    /// from the first page and kept while paging
    private(set) var folderPage: ScrapeResult?
    private(set) var numPages = 0
    private(set) var statusOptions: [StatusStats] = []
    private(set) var groupOptions: [GroupStats] = []
    private(set) var bucketOptions: [PerformanceBucket] = []
    private(set) var resultsError: String?
    private(set) var isLoadingResults = false

    /// folders is the path tree of all results of the selected scrape - it
    /// comes with every page of results, so both always correspond
    private(set) var folders: [Folder] = []

    var resultsQuery: ResultsQuery {
        ResultsQuery(scrape: scrape, prefix: prefix, search: search, status: status, group: group, duration: duration, page: page)
    }

    init(server: Server) {
        self.server = server
        client = ServiceClient(transport: URLSessionTransport(baseURL: server.baseURL))
    }

    func refreshScrapes() async {
        do {
            let scrapes = try await client.getScrapes()
            current = scrapes.current
            complete = scrapes.complete
            connection = .online
        } catch is CancellationError {
            return
        } catch {
            connection = .offline(error.localizedDescription)
        }
    }

    /// pollScrapes refreshes until the calling task is cancelled.
    func pollScrapes(every interval: Swift.Duration) async {
        while !Task.isCancelled {
            await refreshScrapes()
            try? await Task.sleep(for: interval)
        }
    }

    /// brokenLinks of the selected scrape - see loadBrokenLinks
    private(set) var brokenLinks: [BrokenLink] = []
    private(set) var brokenLinksError: String?
    private(set) var isLoadingBrokenLinks = false

    func loadBrokenLinks() async {
        isLoadingBrokenLinks = true
        defer { isLoadingBrokenLinks = false }
        do {
            let brokenLinks = try await client.getBrokenLinks(scrape: scrape.rawValue) ?? []
            guard !Task.isCancelled else { return }
            self.brokenLinks = brokenLinks
            brokenLinksError = nil
            connection = .online
        } catch is CancellationError {
            return
        } catch {
            brokenLinksError = error.localizedDescription
            connection = .offline(error.localizedDescription)
        }
    }

    /// redirects of the selected scrape - see loadRedirects
    private(set) var redirects: [RedirectChain] = []
    private(set) var redirectsError: String?
    private(set) var isLoadingRedirects = false

    func loadRedirects() async {
        isLoadingRedirects = true
        defer { isLoadingRedirects = false }
        do {
            let redirects = try await client.getRedirects(scrape: scrape.rawValue) ?? []
            guard !Task.isCancelled else { return }
            self.redirects = redirects
            redirectsError = nil
            connection = .online
        } catch is CancellationError {
            return
        } catch {
            redirectsError = error.localizedDescription
            connection = .offline(error.localizedDescription)
        }
    }

    /// links are the link counts of every page of the selected scrape
    private(set) var links: [PageLinks] = []
    private(set) var linksError: String?
    private(set) var isLoadingLinks = false

    func loadLinks() async {
        isLoadingLinks = true
        defer { isLoadingLinks = false }
        do {
            let links = try await client.getLinks(scrape: scrape.rawValue) ?? []
            guard !Task.isCancelled else { return }
            self.links = links
            linksError = nil
            connection = .online
        } catch is CancellationError {
            return
        } catch {
            linksError = error.localizedDescription
            connection = .offline(error.localizedDescription)
        }
    }

    /// linkedFrom returns the pages of the selected scrape linking to a url -
    /// totals and groups of all, but at most limit pages
    func linkedFrom(_ url: String, limit: Int) async throws -> LinkedFromPage {
        try await client.getLinkedFrom(scrape: scrape.rawValue, targetURL: url, limit: limit)
    }

    /// isAvailable - the last complete scrape only exists, once one completed;
    /// the current one is always selectable, while walker idles it is empty
    func isAvailable(_ scrape: Scrape) -> Bool {
        switch scrape {
        case .current: true
        case .complete: complete != nil
        }
    }

    /// lookupResult finds the result of the selected scrape for a url, as the
    /// preview browses it - nil, if walker did not scrape it
    func lookupResult(_ url: String) async throws -> ScrapeResult? {
        try await client.getResult(scrape: scrape.rawValue, targetURL: url)
    }

    func loadResults() async {
        isLoadingResults = true
        defer { isLoadingResults = false }
        let filters = Filters(
            scrape: scrape.rawValue,
            prefix: prefix,
            path: search,
            status: status.map { [$0] },
            groups: group.map { [$0] },
            minDur: Duration(rawValue: duration?.from ?? 0),
            maxDur: Duration(rawValue: duration?.to ?? 0)
        )
        do {
            let reply = try await client.getResults(filters: filters, page: page, pageSize: pageSize)
            // a newer query took over while this one was loading
            guard !Task.isCancelled else { return }
            results = reply.results ?? []
            folders = reply.folders ?? []
            if page == 0 || filters.prefix != resultsPrefix {
                folderPage = results.first { $0.targetURL == filters.prefix || $0.targetURL == filters.prefix + "/" }
            }
            resultsPrefix = filters.prefix
            numPages = reply.numPages
            statusOptions = (reply.filterOptions.status ?? []).sorted { $0.code < $1.code }
            groupOptions = reply.filterOptions.groups ?? []
            bucketOptions = reply.filterOptions.buckets ?? []
            resultsError = nil
            connection = .online
        } catch is CancellationError {
            return
        } catch {
            resultsError = error.localizedDescription
            connection = .offline(error.localizedDescription)
        }
    }
}

extension ScrapeResult: Identifiable {
    public var id: String { targetURL }
}
