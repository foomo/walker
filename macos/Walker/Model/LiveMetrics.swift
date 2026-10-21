import Foundation
import Observation

/// LiveMetrics samples the prometheus metrics of a walker into memory, in the
/// background for all servers - a local debugging tool: what the crawl does right
/// now, request rates, latency, walker's own cpu and memory.
///
/// The charts are written as PromQL too (MetricsChart.promQL), so a
/// Prometheus server could answer them later instead of the local samples.
@MainActor
@Observable
final class LiveMetrics {
    struct SeriesKey: Hashable, Sendable {
        let name: String
        let labels: [String: String]
    }

    struct Snapshot: Sendable {
        let date: Date
        let values: [SeriesKey: Double]
    }

    /// retention is how far back the samples go
    static let retention: TimeInterval = 30 * 60

    /// kept are the metrics the charts use - everything else is dropped on arrival
    nonisolated static let kept: Set<String> = [
        "walker_requests_total",
        "walker_scrape_pages",
        "walker_scrape_jobs_open",
        "walker_scrape_request_duration_seconds_bucket",
        "process_cpu_seconds_total",
        // resident memory needs cgo on macOS - the helpers are built without
        // it, Go's own memory statistics are always there
        "process_resident_memory_bytes",
        "go_memstats_sys_bytes",
        "go_memstats_heap_inuse_bytes",
        "go_goroutines",
    ]

    private(set) var snapshots: [Snapshot] = []
    private(set) var error: String?

    @ObservationIgnored private let url: URL

    init(baseURL: URL) {
        url = baseURL.appending(path: "metrics")
    }

    /// sample reads /metrics once
    func sample() async {
        do {
            // a server that does not answer must not stall the sampling of the others
            let (data, response) = try await URLSession.shared.data(for: URLRequest(url: url, timeoutInterval: 5))
            if let http = response as? HTTPURLResponse, http.statusCode != 200 {
                error = "/metrics answered \(http.statusCode)"
                return
            }
            let text = String(decoding: data, as: UTF8.self)
            let values = await Task.detached { Self.values(PromText.parse(text)) }.value
            let now = Date.now
            snapshots.append(Snapshot(date: now, values: values))
            if let first = snapshots.firstIndex(where: { now.timeIntervalSince($0.date) <= Self.retention }), first > 0 {
                snapshots.removeFirst(first)
            }
            error = nil
        } catch is CancellationError {
            return
        } catch {
            self.error = error.localizedDescription
        }
    }

    func clear() {
        snapshots = []
    }

    /// values keeps the charted series - of the walker_scrape_* ones only the current scrape
    nonisolated private static func values(_ samples: [PromSample]) -> [SeriesKey: Double] {
        var values: [SeriesKey: Double] = [:]
        for sample in samples where kept.contains(sample.name) {
            if sample.name.hasPrefix("walker_scrape_"), sample.labels["scrape"] != "current" {
                continue
            }
            values[SeriesKey(name: sample.name, labels: sample.labels)] = sample.value
        }
        return values
    }

    // MARK: - evaluation, the local counterpart of the PromQL of MetricsChart

    /// points evaluates a chart over the samples of the last window
    func points(_ chart: MetricsChart, window: TimeInterval) -> [MetricPoint] {
        let start = Date.now.addingTimeInterval(-window)
        // a rate needs the sample before the window too
        let shown = snapshots.drop { $0.date < start.addingTimeInterval(-60) }
        let points: [MetricPoint]
        switch chart {
        case .requestsByStatus:
            points = rate(shown, "walker_requests_total") { labels in
                Self.statusClass(labels["status"])
            }
        case .requestDuration:
            points = quantiles(shown, "walker_scrape_request_duration_seconds_bucket", [0.5, 0.9, 0.99])
        case .crawlProgress:
            points = gauge(shown, "walker_scrape_pages") { _ in "scraped" }
                + gauge(shown, "walker_scrape_jobs_open") { _ in "open" }
        case .cpu:
            points = rate(shown, "process_cpu_seconds_total") { _ in "cpu" }
        case .memory:
            points = gauge(shown, "go_memstats_sys_bytes") { _ in "from OS" }
                + gauge(shown, "go_memstats_heap_inuse_bytes") { _ in "heap in use" }
                + gauge(shown, "process_resident_memory_bytes") { _ in "resident" }
        case .goroutines:
            points = gauge(shown, "go_goroutines") { _ in "goroutines" }
        }
        return points.filter { $0.date >= start }
    }

    /// statusClass is 2xx … 5xx, or failed for requests without a response
    nonisolated static func statusClass(_ status: String?) -> String {
        guard let code = status.flatMap(Int.init), code > 0 else { return "failed" }
        return "\(code / 100)xx"
    }

    /// gauge sums the series of a metric per snapshot, by a series name of their labels
    private func gauge(_ snapshots: ArraySlice<Snapshot>, _ name: String, by series: ([String: String]) -> String) -> [MetricPoint] {
        snapshots.flatMap { snapshot in
            var sums: [String: Double] = [:]
            for (key, value) in snapshot.values where key.name == name {
                sums[series(key.labels), default: 0] += value
            }
            return sums.map { MetricPoint(date: snapshot.date, series: $0.key, value: $0.value) }
        }
    }

    /// rate is the per second increase of counters between two snapshots,
    /// summed by series - like rate() and sum by () in PromQL
    private func rate(_ snapshots: ArraySlice<Snapshot>, _ name: String, by series: ([String: String]) -> String) -> [MetricPoint] {
        zip(snapshots, snapshots.dropFirst()).flatMap { previous, snapshot in
            let seconds = snapshot.date.timeIntervalSince(previous.date)
            guard seconds > 0 else { return [MetricPoint]() }
            var sums: [String: Double] = [:]
            for (key, value) in snapshot.values where key.name == name {
                sums[series(key.labels), default: 0] += Self.increase(from: previous.values[key], to: value)
            }
            return sums.map { MetricPoint(date: snapshot.date, series: $0.key, value: $0.value / seconds) }
        }
    }

    /// quantiles of the requests between two snapshots, over all groups - like
    /// histogram_quantile(q, sum by (le) (rate(…_bucket))). The histogram covers
    /// the cycle so far, the increase is what was requested meanwhile.
    private func quantiles(_ snapshots: ArraySlice<Snapshot>, _ name: String, _ qs: [Double]) -> [MetricPoint] {
        zip(snapshots, snapshots.dropFirst()).flatMap { previous, snapshot in
            var buckets: [Double: Double] = [:]
            for (key, value) in snapshot.values where key.name == name {
                guard let le = key.labels["le"].flatMap({ $0 == "+Inf" ? .infinity : Double($0) }) else { continue }
                buckets[le, default: 0] += Self.increase(from: previous.values[key], to: value)
            }
            let sorted = buckets.sorted { $0.key < $1.key }
            guard let total = sorted.last?.value, total > 0 else { return [MetricPoint]() }
            return qs.compactMap { q in
                Self.quantile(q, sorted, total: total).map {
                    MetricPoint(date: snapshot.date, series: "p\(Int(q * 100))", value: $0)
                }
            }
        }
    }

    /// increase of a counter - a smaller value means it was reset, e.g. by a new cycle
    nonisolated private static func increase(from previous: Double?, to value: Double) -> Double {
        guard let previous else { return 0 }
        return value >= previous ? value - previous : value
    }

    /// quantile interpolates within the bucket like histogram_quantile - in the
    /// open last bucket it is the bound of the one before
    nonisolated private static func quantile(_ q: Double, _ buckets: [(key: Double, value: Double)], total: Double) -> Double? {
        let rank = q * total
        var lower = 0.0, below = 0.0
        for (le, count) in buckets {
            if count >= rank {
                if le.isInfinite { return lower }
                let inBucket = count - below
                return inBucket > 0 ? lower + (le - lower) * (rank - below) / inBucket : le
            }
            lower = le
            below = count
        }
        return nil
    }
}

struct MetricPoint: Identifiable, Hashable {
    let date: Date
    let series: String
    let value: Double

    var id: String { "\(series)-\(date.timeIntervalSinceReferenceDate)" }
}

/// MetricsChart is one chart of the live metrics - one measure, one axis
enum MetricsChart: String, CaseIterable, Identifiable {
    case requestsByStatus
    case requestDuration
    case crawlProgress
    case cpu
    case memory
    case goroutines

    var id: Self { self }

    var title: String {
        switch self {
        case .requestsByStatus: "Requests per second"
        case .requestDuration: "Request duration"
        case .crawlProgress: "Crawl progress"
        case .cpu: "Walker CPU"
        case .memory: "Walker memory"
        case .goroutines: "Goroutines"
        }
    }

    /// promQL is the query a Prometheus server answers the chart with
    var promQL: String {
        switch self {
        case .requestsByStatus:
            #"sum by (status) (rate(walker_requests_total[1m]))"#
        case .requestDuration:
            #"histogram_quantile(0.9, sum by (le) (rate(walker_scrape_request_duration_seconds_bucket{scrape="current"}[1m])))"#
        case .crawlProgress:
            #"sum(walker_scrape_pages{scrape="current"}), walker_scrape_jobs_open{scrape="current"}"#
        case .cpu:
            #"rate(process_cpu_seconds_total[1m])"#
        case .memory:
            #"go_memstats_sys_bytes, go_memstats_heap_inuse_bytes, process_resident_memory_bytes"#
        case .goroutines:
            #"go_goroutines"#
        }
    }
}
