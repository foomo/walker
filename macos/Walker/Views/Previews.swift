#if DEBUG
import SwiftUI

/// Fixtures to tune the look in the Xcode canvas without a running walker.
enum PreviewData {
    private static let buckets: [(name: String, fromMS: Int64, toMS: Int64)] = [
        ("awesome", 0, 50),
        ("great", 50, 100),
        ("ok, google loves you", 100, 200),
        ("not too good, but still ok", 200, 300),
        ("not great", 300, 500),
        ("bad, users start to feel a real difference", 500, 1000),
        ("really bad, you are loosing users", 1000, 3000),
        ("ouch this seems broken", 3000, 5000),
        ("catastrophic you site seems to be down", 5000, 10000),
        ("end of the world - this must not happen", 10000, 3_600_000),
    ]

    private static func ms(_ value: Int64) -> Duration {
        Duration(rawValue: value * 1_000_000)
    }

    static func group(_ name: String, _ pages: [Int], p50: Int64, p90: Int64, max: Int64) -> GroupPerformance {
        GroupPerformance(
            group: name,
            pages: pages.reduce(0, +),
            buckets: zip(buckets, pages).map { bucket, pages in
                PerformanceBucket(name: bucket.name, from: ms(bucket.fromMS), to: ms(bucket.toMS), pages: pages)
            },
            p50: ms(p50),
            p90: ms(p90),
            max: ms(max)
        )
    }

    static let performance = [
        group("content", [310, 120, 40, 12, 6, 2, 0, 0, 0, 0], p50: 38, p90: 110, max: 640),
        group("category", [20, 80, 140, 60, 30, 12, 4, 1, 0, 0], p50: 160, p90: 420, max: 3400),
        group("product", [5, 30, 260, 180, 90, 40, 18, 3, 1, 1], p50: 230, p90: 560, max: 12_800),
    ]

    static let summary = ScrapeSummary(
        running: false,
        started: .now.addingTimeInterval(-1800),
        completed: .now.addingTimeInterval(-300),
        cycleStart: .now.addingTimeInterval(-1800),
        cycleDeadline: .now.addingTimeInterval(1800),
        nextCycle: .now.addingTimeInterval(1800),
        pages: 1_472,
        jobsOpen: 0,
        statusCodes: [200: 1_430, 301: 22, 404: 17, 500: 3],
        scrapeSpeed: 0,
        scrapeSpeedAverage: 1.6,
        performance: performance
    )
}

#Preview("Performance") {
    ScrollView {
        PerformanceDetails(summary: PreviewData.summary)
            .padding()
    }
    .frame(width: 900, height: 760)
}

#Preview("Results sidebar") {
    @Previewable @State var prefix = "https://example.com/shop"
    @Previewable @State var status: Int? = 404
    @Previewable @State var group: String?
    @Previewable @State var duration: ServerModel.DurationRange?
    ResultsSidebar(
        folders: [
            Folder(name: "example.com", prefix: "https://example.com", pages: 1_472, errors: 20, folders: [
                Folder(name: "blog", prefix: "https://example.com/blog", pages: 212, errors: 0),
                Folder(name: "shop", prefix: "https://example.com/shop", pages: 1_180, errors: 19, folders: [
                    Folder(name: "men", prefix: "https://example.com/shop/men", pages: 540, errors: 4),
                    Folder(name: "women", prefix: "https://example.com/shop/women", pages: 610, errors: 15),
                ]),
            ]),
        ],
        statusOptions: [StatusStats(code: 200, count: 1_430), StatusStats(code: 301, count: 22), StatusStats(code: 404, count: 17), StatusStats(code: 500, count: 3)],
        groupOptions: [GroupStats(group: "category", count: 230), GroupStats(group: "content", count: 490), GroupStats(group: "product", count: 752)],
        bucketOptions: PreviewData.summary.mergedBuckets,
        prefix: $prefix,
        status: $status,
        group: $group,
        duration: $duration
    )
    .frame(width: 280, height: 560)
}

#Preview("Result rows") {
    let rows: [(code: Int, path: String, group: String, ms: Int64, title: String)] = [
        (200, "/damen/damen-accessoires", "category", 184, "Damen Accessoires online kaufen | Example Shop"),
        (200, "/damen/mode/jeans/acme-jeans-wide-leg-relaxed-fit-1234567890", "product", 612, "ACME Jeans Wide Leg Relaxed Fit | Example Shop"),
        (404, "/damen/damen-accessoires/taschen-alt", "", 38, ""),
        (0, "/herren/schuhe", "category", 10_000, ""),
    ]
    List(rows, id: \.path) { row in
        ResultRow(
            result: ScrapeResult(
                targetURL: "https://shop.example" + row.path,
                error: "",
                code: row.code,
                status: "",
                contentType: "text/html",
                length: 0,
                duration: Duration(rawValue: row.ms * 1_000_000),
                time: .now,
                structure: Structure(title: row.title, description: "", robots: "", canonical: "", linkPrev: "", linkNext: ""),
                data: .null,
                group: row.group
            ),
            path: row.path
        )
    }
    .frame(width: 520, height: 260)
}

#Preview("Crawl progress") {
    @Previewable @State var section = ServerDetailView.Section.results
    let running = ScrapeSummary(
        running: true,
        started: .now.addingTimeInterval(-420),
        pages: 612,
        jobsOpen: 590,
        statusCodes: [200: 540, 301: 30, 404: 28, 500: 8, 0: 6],
        scrapeSpeed: 1.4,
        scrapeSpeedAverage: 1.5
    )
    Picker("Section", selection: $section) {
        ForEach(ServerDetailView.Section.allCases) { section in
            Text(section.rawValue).tag(section)
        }
    }
    .pickerStyle(.segmented)
    .labelsHidden()
    .background {
        CrawlProgressBar(summary: running)
    }
    .help(CrawlProgressBar.help(for: running))
    .padding()
}

#Preview("Headings") {
    Form {
        Section("Headings") {
            HeadingOutline(headings: [
                Heading(level: 1, text: "Bestseller von ACME für Damen"),
                Heading(level: 2, text: "Neu eingetroffen"),
                Heading(level: 3, text: "Jeans"),
                Heading(level: 3, text: "Hosen"),
                Heading(level: 2, text: "Marken"),
                Heading(level: 4, text: "ACME Jeans Parla"),
                Heading(level: 3, text: "  "),
                Heading(level: 5, text: "Newsletter abonnieren"),
                Heading(level: 1, text: "Service"),
            ])
        }
    }
    .formStyle(.grouped)
    .frame(width: 640, height: 420)
}

#Preview("Strip") {
    PerformanceStrip(shares: PreviewData.summary.mergedBuckets.shares())
        .padding()
        .frame(width: 480)
}

#Preview("Scrape card") {
    ScrapeCard(title: "Last complete scrape", systemImage: "checkmark.circle", summary: PreviewData.summary) {
        Text("No scrape has completed yet.")
    }
    .padding()
    .frame(width: 720)
}
#endif
