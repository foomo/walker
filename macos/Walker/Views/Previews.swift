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

    private static func ms(_ value: Int64) -> GoTime.Duration {
        GoTime.Duration(rawValue: value * 1_000_000)
    }

    static func group(_ name: String, _ pages: [Int], p50: Int64, p90: Int64, max: Int64) -> WalkerVO.GroupPerformance {
        WalkerVO.GroupPerformance(
            group: name,
            pages: pages.reduce(0, +),
            buckets: zip(buckets, pages).map { bucket, pages in
                WalkerVO.PerformanceBucket(name: bucket.name, from: ms(bucket.fromMS), to: ms(bucket.toMS), pages: pages)
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

    static let summary = WalkerVO.ScrapeSummary(
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
            WalkerVO.Folder(name: "example.com", prefix: "https://example.com", pages: 1_472, errors: 20, folders: [
                WalkerVO.Folder(name: "blog", prefix: "https://example.com/blog", pages: 212, errors: 0),
                WalkerVO.Folder(name: "shop", prefix: "https://example.com/shop", pages: 1_180, errors: 19, folders: [
                    WalkerVO.Folder(name: "men", prefix: "https://example.com/shop/men", pages: 540, errors: 4),
                    WalkerVO.Folder(name: "women", prefix: "https://example.com/shop/women", pages: 610, errors: 15),
                ]),
            ]),
        ],
        statusOptions: [Walker.StatusStats(code: 200, count: 1_430), Walker.StatusStats(code: 301, count: 22), Walker.StatusStats(code: 404, count: 17), Walker.StatusStats(code: 500, count: 3)],
        groupOptions: [Walker.GroupStats(group: "category", count: 230), Walker.GroupStats(group: "content", count: 490), Walker.GroupStats(group: "product", count: 752)],
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
            result: WalkerVO.ScrapeResult(
                targetURL: "https://shop.example" + row.path,
                error: "",
                code: row.code,
                validationError: "",
                status: "",
                contentType: "text/html",
                length: 0,
                duration: GoTime.Duration(rawValue: row.ms * 1_000_000),
                ttfb: GoTime.Duration(rawValue: row.ms * 800_000),
                time: .now,
                structure: WalkerVO.Structure(title: row.title, description: "", robots: "", canonical: "", linkPrev: "", linkNext: ""),
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
    let running = WalkerVO.ScrapeSummary(
        running: true,
        started: .now.addingTimeInterval(-420),
        pages: 612,
        jobsOpen: 590,
        statusCodes: [200: 540, 301: 30, 404: 28, 500: 8, 0: 6],
        scrapeSpeed: 1.4,
        scrapeSpeedAverage: 1.5
    )
    Picker("Section", selection: $section) {
        ForEach(ServerDetailView.Section.tabs) { section in
            Text(section.rawValue).tag(section)
        }
    }
    .pickerStyle(.segmented)
    .labelsHidden()
    .padding(.bottom, 40)
    .frame(width: 900)
    .overlay(alignment: .bottom) {
        StatusBar(current: running, complete: PreviewData.summary, connection: .online)
    }
    .padding()
}

#Preview("Document outline") {
    func element(_ type: String, _ selector: String, text: String = "", label: String = "", links: [SemanticScrapeVO.Link]? = nil, src: String = "", _ children: [SemanticScrapeVO.DocumentElement]? = nil) -> SemanticScrapeVO.DocumentElement {
        SemanticScrapeVO.DocumentElement(elementType: SemanticScrapeVO.ElementType(rawValue: type), selector: selector, text: text, label: label, links: links, src: src, children: children)
    }
    func link(_ text: String, _ href: String) -> SemanticScrapeVO.Link {
        SemanticScrapeVO.Link(text: text, href: href, selector: href)
    }
    return Form {
        Section("Document outline") {
            SemanticOutline(elements: [
                element("header", "h", [
                    element("nav", "hn", label: "Main", links: [link("Home", "/"), link("Guides", "/guides"), link("", "/search")]),
                ]),
                element("main", "m", [
                    element("article", "a", [
                        element("h1", "a1", text: "Keeping a sourdough starter alive"),
                        element("text", "a2", text: "October 21, 2026, by Lena Huber", links: [link("Lena Huber", "/authors/lena")]),
                        element("section", "s1", label: "Feeding", [
                            element("h2", "s1h", text: "Feeding"),
                            element("text", "s1p", text: "Discard half, then add equal weights of flour and water. Do this once a day at room temperature."),
                            element("figure", "s1f", text: "A starter at its peak, about six hours after feeding.", [
                                element("img", "s1i", text: "A jar of bubbly starter", src: "https://example.com/starter.jpg"),
                            ]),
                            element("img", "s1d", src: "https://example.com/divider.png"),
                        ]),
                        element("ul", "l", [
                            element("li", "l1", text: "It doubles within 4–8 hours."),
                            element("li", "l2", text: "It smells tangy, not sharp.", [
                                element("ol", "l2o", [element("li", "l2o1", text: "Sniff"), element("li", "l2o2", text: "Taste")]),
                            ]),
                        ]),
                        element("table", "t", text: "Feeding schedule", [
                            element("tr", "t1", [element("th", "t1a", text: "Day"), element("th", "t1b", text: "Flour")]),
                            element("tr", "t2", [element("td", "t2a", text: "Monday"), element("td", "t2b", text: "50 g")]),
                            element("tr", "t3", [element("td", "t3a", text: "Tuesday"), element("td", "t3b", text: "50 g", links: [link("50 g", "/flour")])]),
                        ]),
                        element("section", "s2", label: "Signs it’s healthy", [
                            element("h2", "s2h", text: "Signs it’s healthy"),
                            element("h4", "s2h4", text: "Bubbles"),
                        ]),
                        element("section", "s3", [element("h2", "s3h", text: "  ")]),
                    ]),
                    element("aside", "b", label: "Related guides", [element("h2", "bh", text: "Related guides")]),
                    element("div", "c", [element("h1", "ch", text: "Newsletter")]),
                ]),
                element("footer", "f", [
                    element("nav", "fn", label: "Footer", links: [link("Privacy", "/privacy"), link("Contact", "/contact")]),
                ]),
            ])
        }
    }
    .formStyle(.grouped)
    .frame(width: 640, height: 820)
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
extension PreviewData {
    static func trace(_ kind: String, _ label: String, _ status: HTMLSchemaVO.TraceStatus, points: Int = 0, max: Int = 0, penalty: Int = 0, rules: String = "", found: Bool = true, findings: [HTMLSchemaVO.Finding] = [], children: [HTMLSchemaVO.TraceNode] = []) -> HTMLSchemaVO.TraceNode {
        HTMLSchemaVO.TraceNode(kind: kind, label: label, path: label, source: "groups/content/landing.shape.html:12", rules: rules, status: status, points: points, max: max, penalty: penalty, locators: ["html > body"], findings: findings, children: children, found: found, via: "")
    }

    static let traceTree = trace("page", "/groups/content/landing", .ok, points: 112, max: 165, penalty: 5, children: [
        trace("element", "html", .ok, points: 112, max: 165, penalty: 5, rules: "attr-lang=one-of:en|de", children: [
            trace("element", "head", .ok, points: 29, max: 35, children: [
                trace("element", "title", .ok, points: 10, max: 10, rules: "score=10 penalty=20 min-length=30 max-length=60"),
                trace("element", "meta[property=og:image]", .absent, max: 3, rules: "property=og:image optional score=3", found: false),
            ]),
            trace("element", "body", .ok, points: 83, max: 130, penalty: 5, children: [
                trace("choice", "val:choice", .ok, points: 53, max: 100, penalty: 5, rules: "min=2 max=12 score-max=100", children: [
                    trace("component", "teaser[1]", .ok, points: 10, max: 10),
                    trace("component", "teaser[2]", .ok, points: 8, max: 10, penalty: 5, children: [
                        trace("element", "img", .error, max: 2, penalty: 5, rules: "optional score=2 penalty=5 attr-alt=min-length:5", findings: [
                            HTMLSchemaVO.Finding(severity: .error, penalty: 5, rule: "attr:min-length", path: "html/body/main/teaser[2]/img@alt", locator: "html > body > img", message: "got 0, expected at least 5", component: "/components/teaser", source: "components/teaser.shape.html:7"),
                        ]),
                    ]),
                ]),
            ]),
        ]),
    ])
}

#Preview("Schema tree") {
    @Previewable @State var selection: TraceItem.ID?
    @Previewable @State var collapsed: Set<TraceItem.ID> = []
    SchemaTreeView(items: [TraceItem(node: PreviewData.traceTree, parentID: "")], selection: $selection, collapsed: $collapsed)
        .frame(width: 440, height: 420)
}
#endif
