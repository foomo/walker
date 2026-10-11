import SwiftUI

/// CrawlProgressBar shows the progress of a running scrape: the crawled pages,
/// colored by status class, as a share of all known pages - crawled and open.
struct CrawlProgressBar: View {
    let summary: ScrapeSummary?

    /// Segment is one status class of the crawled pages
    struct Segment: Identifiable, Equatable {
        let id: String
        let color: Color
        let pages: Int
    }

    /// statusClasses in the order they are drawn - like StatusClass.color
    private static let statusClasses: [(id: String, code: Int)] = [
        ("2xx", 200), ("3xx", 300), ("4xx", 400), ("5xx", 500), ("failed", 0),
    ]

    static func segments(of summary: ScrapeSummary?) -> [Segment] {
        var pages: [String: Int] = [:]
        for (code, count) in summary?.statusCodes ?? [:] {
            let id = switch code {
            case 200..<300: "2xx"
            case 300..<400: "3xx"
            case 400..<500: "4xx"
            case 500...: "5xx"
            default: "failed"
            }
            pages[id, default: 0] += count
        }
        return statusClasses.compactMap { statusClass in
            guard let count = pages[statusClass.id], count > 0 else { return nil }
            return Segment(id: statusClass.id, color: StatusClass.color(for: statusClass.code), pages: count)
        }
    }

    private var total: Int {
        guard let summary else { return 0 }
        return summary.pages + summary.jobsOpen
    }

    var body: some View {
        GeometryReader { geometry in
            HStack(spacing: 0) {
                if total > 0 {
                    ForEach(Self.segments(of: summary)) { segment in
                        Rectangle()
                            .fill(segment.color.opacity(0.35))
                            .frame(width: geometry.size.width * CGFloat(segment.pages) / CGFloat(total))
                    }
                }
                Spacer(minLength: 0)
            }
        }
        .clipShape(.capsule)
        .animation(.smooth, value: Self.segments(of: summary))
        .animation(.smooth, value: total)
        .allowsHitTesting(false)
    }

    /// help describes the progress, e.g. "412 of 1,203 pages crawled · 380 × 200, 12 × 404"
    static func help(for summary: ScrapeSummary?) -> String {
        guard let summary, summary.running else { return "No scrape is running" }
        let codes = (summary.statusCodes ?? [:])
            .sorted { $0.value > $1.value }
            .map { "\($0.value.formatted()) × \(StatusClass.label(for: $0.key))" }
            .joined(separator: ", ")
        let progress = "\(summary.pages.formatted()) of \((summary.pages + summary.jobsOpen).formatted()) pages crawled"
        return codes.isEmpty ? progress : "\(progress) · \(codes)"
    }
}
