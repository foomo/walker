import SwiftUI

// The generated client defines `GoTime.Duration` for go's time.Duration
// (nanoseconds) - a bare `Duration` is Swift.Duration.
extension GoTime.Duration {
    var swiftDuration: Swift.Duration { .nanoseconds(rawValue) }

    var formatted: String {
        swiftDuration.formatted(.units(allowed: [.seconds, .milliseconds], width: .abbreviated, maximumUnitCount: 2))
    }
}

extension WalkerVO.ScrapeSummary {
    /// elapsed is the duration of a completed scrape, or the time since a running one started
    func elapsed(now: Date = .now) -> Swift.Duration? {
        guard let started else { return nil }
        return .seconds((completed ?? now).timeIntervalSince(started))
    }

    var sortedStatusCodes: [(code: Int, count: Int)] {
        (statusCodes ?? [:]).sorted { $0.key < $1.key }.map { (code: $0.key, count: $0.value) }
    }
}

enum StatusClass {
    static func color(for code: Int) -> Color {
        switch code {
        case 200..<300: .green
        case 300..<400: .blue
        case 400..<500: .orange
        case 500...: .red
        default: .gray // 0 => the request failed
        }
    }

    static func label(for code: Int) -> String {
        code == 0 ? "failed" : String(code)
    }
}

// MARK: - performance buckets

/// BucketShare is one performance bucket prepared for rendering.
struct BucketShare: Identifiable, Equatable {
    let index: Int
    let count: Int
    let name: String
    let from: GoTime.Duration
    let to: GoTime.Duration
    let pages: Int
    let total: Int

    var id: Int { index }
    var share: Double { total > 0 ? Double(pages) / Double(total) : 0 }
    var color: Color { BucketShare.color(index: index, count: count) }

    /// label is a compact range like "< 50 ms", "1–3 s" or "≥ 10 s"
    var label: String {
        if index == 0 {
            return "< \(Self.format(to))"
        }
        if index == count - 1 {
            return "≥ \(Self.format(from))"
        }
        let fromSeconds = from.rawValue >= 1_000_000_000
        let toSeconds = to.rawValue >= 1_000_000_000
        if fromSeconds == toSeconds {
            return "\(Self.number(from))–\(Self.format(to))"
        }
        return "\(Self.format(from))–\(Self.format(to))"
    }

    /// color runs from green over yellow and orange to red across the buckets
    static func color(index: Int, count: Int) -> Color {
        let stops: [(Double, Color)] = [
            (0, .green),
            (0.35, .yellow),
            (0.6, .orange),
            (0.85, .red),
            (1, Color(red: 0.55, green: 0.05, blue: 0.25)),
        ]
        let t = count > 1 ? Double(index) / Double(count - 1) : 0
        for (lower, upper) in zip(stops, stops.dropFirst()) where t <= upper.0 {
            return lower.1.mix(with: upper.1, by: (t - lower.0) / (upper.0 - lower.0))
        }
        return stops[stops.count - 1].1
    }

    private static func number(_ d: GoTime.Duration) -> String {
        d.rawValue >= 1_000_000_000 ? String(d.rawValue / 1_000_000_000) : String(d.rawValue / 1_000_000)
    }

    private static func format(_ d: GoTime.Duration) -> String {
        d.rawValue >= 1_000_000_000 ? "\(number(d)) s" : "\(number(d)) ms"
    }
}

extension Array where Element == WalkerVO.PerformanceBucket {
    func shares() -> [BucketShare] {
        let total = reduce(0) { $0 + $1.pages }
        return enumerated().map { index, bucket in
            BucketShare(index: index, count: count, name: bucket.name, from: bucket.from, to: bucket.to, pages: bucket.pages, total: total)
        }
    }

    /// bucketIndex finds the bucket of a duration - [from, to)
    func bucketIndex(of d: GoTime.Duration) -> Int {
        firstIndex { d.rawValue >= $0.from.rawValue && d.rawValue < $0.to.rawValue } ?? Swift.max(count - 1, 0)
    }
}

extension WalkerVO.GroupPerformance {
    var shares: [BucketShare] { (buckets ?? []).shares() }

    func color(of d: GoTime.Duration) -> Color {
        let buckets = buckets ?? []
        return BucketShare.color(index: buckets.bucketIndex(of: d), count: buckets.count)
    }
}

extension WalkerVO.ScrapeSummary {
    /// mergedBuckets sums up the buckets of all groups
    var mergedBuckets: [WalkerVO.PerformanceBucket] {
        var merged: [WalkerVO.PerformanceBucket] = []
        for group in performance ?? [] {
            for (index, bucket) in (group.buckets ?? []).enumerated() {
                if index < merged.count {
                    merged[index].pages += bucket.pages
                } else {
                    merged.append(bucket)
                }
            }
        }
        return merged
    }

    /// share(within:) is the share of pages faster than d - exact on bucket boundaries
    func share(within d: GoTime.Duration) -> Double? {
        let buckets = mergedBuckets
        let total = buckets.reduce(0) { $0 + $1.pages }
        guard total > 0 else { return nil }
        let within = buckets.filter { $0.to.rawValue <= d.rawValue }.reduce(0) { $0 + $1.pages }
        return Double(within) / Double(total)
    }
}

// MARK: - paths

extension WalkerVO.ScrapeResult {
    /// path is the target url without scheme and host, e.g. "/shop/a?color=red"
    var path: String { targetURL.urlPath }

    /// displayPath strips a folder - "https://example.com/shop" turns .../shop/a
    /// into "a" and the folder's own page into "./" - otherwise it is the path
    func displayPath(relativeTo folder: String) -> String {
        guard !folder.isEmpty, targetURL.isInFolder(folder) else { return path }
        var relative = targetURL.dropFirst(folder.count)
        if relative.first == "/" {
            relative = relative.dropFirst()
        }
        if relative.isEmpty || relative.first == "?" || relative.first == "#" {
            return "./" + relative
        }
        return String(relative)
    }
}

extension String {
    /// urlPath is the url without scheme and host, e.g. "/shop/a?color=red"
    var urlPath: String {
        guard let schemeEnd = range(of: "://") else { return self }
        let rest = self[schemeEnd.upperBound...]
        guard let pathStart = rest.firstIndex(where: { "/?#".contains($0) }) else { return "/" }
        return String(rest[pathStart...])
    }

    /// isInFolder mirrors vo.InFolder: https://example.com/shop contains .../shop,
    /// .../shop/, .../shop/a and .../shop?page=2 - but not .../shopping
    func isInFolder(_ folder: String) -> Bool {
        guard hasPrefix(folder) else { return false }
        guard let next = dropFirst(folder.count).first else { return true }
        return "/?#".contains(next)
    }
}
