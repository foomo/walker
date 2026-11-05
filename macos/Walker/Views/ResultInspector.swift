import SwiftUI

/// ResultLookup finds the result for a url the preview navigated to - nil, if
/// walker did not scrape it
typealias ResultLookup = @MainActor (String) async throws -> WalkerVO.ScrapeResult?

struct ResultInspector: View {
    let result: WalkerVO.ScrapeResult
    let lookup: ResultLookup

    var body: some View {
        Form {
            BrowsableResult(start: result, lookup: lookup)
        }
        .formStyle(.grouped)
        .textSelection(.enabled)
        // a new selection starts over - browsing keeps the preview running
        .id(result.targetURL)
    }
}

/// BrowsableResult previews a result and follows the preview: when the page
/// navigates, it shows the result of the new url.
struct BrowsableResult: View {
    let start: WalkerVO.ScrapeResult
    let lookup: ResultLookup
    /// highlight outlines links to this url in the preview
    var highlight: String?

    @State private var browsedURL: String?
    @State private var browsed = Browsed.start
    @State private var preview = PreviewController()

    enum Browsed {
        case start
        case loading(String)
        case found(WalkerVO.ScrapeResult)
        case notScraped(String)
        case failed(String, String)
    }

    /// shown is the result the inspector shows - the start, or where the preview went
    private var shown: WalkerVO.ScrapeResult? {
        switch browsed {
        case .start: start
        case let .found(result): result
        case .loading, .notScraped, .failed: nil
        }
    }

    var body: some View {
        Group {
            // title, description and url on top - of the page being shown
            Section {
                if let shown {
                    ResultHeader(title: shown.structure.title, description: shown.structure.description, url: shown.targetURL)
                } else if let browsedURL {
                    ResultHeader(title: "", description: "", url: browsedURL)
                }
            }
            if let previewURL = start.previewURL {
                Section {
                    PagePreview(url: previewURL, highlight: highlight, controller: preview) { url in
                        navigated(to: url.absoluteString)
                    }
                    .task(id: browsedURL) {
                        await lookUp()
                    }
                }
            }
            switch browsed {
            case .start:
                ResultSections(result: start, preview: preview)
            case let .found(result):
                ResultSections(result: result, preview: preview)
            case .loading:
                Section("Response") {
                    ProgressView()
                        .controlSize(.small)
                        .frame(maxWidth: .infinity)
                }
            case .notScraped:
                Section("Response") {
                    Label("Walker did not scrape this page in the selected scrape.", systemImage: "questionmark.circle")
                        .foregroundStyle(.secondary)
                }
            case let .failed(_, message):
                Section("Response") {
                    Label(message, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.red)
                }
            }
        }
    }

    private func navigated(to url: String) {
        // the start page loading - including its redirects - is no navigation
        if browsedURL == nil && url.comparableURL == start.targetURL.comparableURL {
            return
        }
        if url != browsedURL {
            browsedURL = url
        }
    }

    private func lookUp() async {
        guard let url = browsedURL else { return }
        browsed = .loading(url)
        do {
            let result = try await lookup(url)
            guard !Task.isCancelled else { return }
            if let result {
                browsed = result.targetURL == start.targetURL ? .start : .found(result)
            } else {
                browsed = .notScraped(url)
            }
        } catch is CancellationError {
            return
        } catch {
            browsed = .failed(url, error.localizedDescription)
        }
    }
}

extension WalkerVO.ScrapeResult {
    /// previewURL is set for html pages that loaded - not for failed requests,
    /// images, json or the like
    var previewURL: URL? {
        guard code != 0, error.isEmpty else { return nil }
        let contentType = contentType.lowercased()
        guard contentType.isEmpty || contentType.contains("html") else { return nil }
        return URL(string: targetURL)
    }
}

extension String {
    /// comparableURL strips the fragment and credentials - like the server
    /// does to find the result of a url
    var comparableURL: String {
        var url = self
        if let hash = url.firstIndex(of: "#") {
            url = String(url[..<hash])
        }
        guard let schemeEnd = url.range(of: "://") else { return url }
        let hostEnd = url[schemeEnd.upperBound...].firstIndex(where: { $0 == "/" || $0 == "?" }) ?? url.endIndex
        if let at = url[schemeEnd.upperBound..<hostEnd].lastIndex(of: "@") {
            url.removeSubrange(schemeEnd.upperBound...at)
        }
        return url
    }
}

/// ResultSections are the form sections of a result - shared by the result
/// and the folder inspector.
struct ResultSections: View {
    let result: WalkerVO.ScrapeResult
    /// preview shows the page of the result - hovering the outline highlights it there
    var preview: PreviewController?

    var body: some View {
        Group {
            // right below the preview: the outline and the links of the page
            if result.previewURL != nil {
                SemanticOutlineSection(url: result.targetURL, preview: preview)
            }

            Section("Links") {
                LinksDisclosure(links: result.normalizedLinks ?? result.links ?? [:])
            }

            Section("Response") {
                LabeledContent("Status") {
                    // the status text contains the code already, e.g. "200 OK"
                    Text(result.status.isEmpty ? StatusClass.label(for: result.code) : result.status)
                        .foregroundStyle(StatusClass.color(for: result.code))
                }
                LabeledContent("Duration", value: result.duration.formatted)
                LabeledContent("Time to first byte", value: result.ttfb.formatted)
                LabeledContent("Scraped", value: result.time.formatted(.dateTime))
                LabeledContent("Group", value: result.group.isEmpty ? "–" : result.group)
                LabeledContent("Content type", value: result.contentType)
                LabeledContent("Size", value: result.length.formatted(.byteCount(style: .file)))
                if !result.error.isEmpty {
                    LabeledContent("Error") {
                        Text(result.error)
                            .foregroundStyle(.red)
                    }
                }
            }

            if let redirects = result.redirects, !redirects.isEmpty {
                Section("Redirects") {
                    ForEach(Array(redirects.enumerated()), id: \.offset) { _, redirect in
                        LabeledContent(String(redirect.code)) {
                            URLValue(redirect.url, isLink: true)
                        }
                    }
                }
            }

            Section("Structure") {
                LabeledContent("Canonical") {
                    URLValue(result.structure.canonical)
                }
                LabeledContent("Robots", value: result.structure.robots)
            }


            if let validations = result.validations, !validations.isEmpty {
                Section("Validations") {
                    ForEach(Array(validations.enumerated()), id: \.offset) { _, validation in
                        LabeledContent(validation.level.rawValue, value: validation.group.isEmpty ? validation.message : "\(validation.group): \(validation.message)")
                    }
                }
            }

            if let report = result.validationReport {
                Section("Shape validation") {
                    LabeledContent("Shape", value: report.shape)
                    if let score = report.score {
                        LabeledContent("Score", value: score.summary)
                    }
                    // score and penalty are separate measures, never combined
                    LabeledContent("Penalty", value: report.penalty.formatted())
                    ForEach(Array((report.findings ?? []).compactMap { $0 }.enumerated()), id: \.offset) { _, finding in
                        LabeledContent("\(finding.severity.rawValue) \(finding.rule)") {
                            Text("\(finding.message) (penalty \(finding.penalty))")
                        }
                        .help("\(finding.path)\n\(finding.locator)\n\(finding.source)")
                    }
                }
            } else if !result.validationError.isEmpty {
                Section("Shape validation") {
                    Text(result.validationError)
                }
            }
        }
    }
}

/// URLValue shows a url in one line, shortened in the middle - urls have no
/// spaces to wrap at, at full length they would widen the inspector.
struct URLValue: View {
    let url: String
    let isLink: Bool

    init(_ url: String, isLink: Bool = false) {
        self.url = url
        self.isLink = isLink
    }

    var body: some View {
        Group {
            if isLink, let destination = URL(string: url) {
                Link(destination: destination) {
                    text
                }
            } else {
                text
            }
        }
        .help(url)
    }

    private var text: some View {
        Text(verbatim: url.isEmpty ? "–" : url)
            .lineLimit(1)
            .truncationMode(.middle)
    }
}

/// ResultHeader is the top of the inspector - title, description and url.
struct ResultHeader: View {
    let title: String
    let description: String
    let url: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(verbatim: title.isEmpty ? "Untitled" : title)
                .font(.title3.weight(.semibold))
                .foregroundStyle(title.isEmpty ? .secondary : .primary)
                .fixedSize(horizontal: false, vertical: true)
            if !description.isEmpty {
                Text(verbatim: description)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            URLValue(url, isLink: true)
                .font(.callout)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 2)
    }
}

/// LinksDisclosure lists the links of a page, collapsed - pages easily have hundreds.
struct LinksDisclosure: View {
    let links: [String: Int]

    @State private var isExpanded = false

    var body: some View {
        DisclosureGroup(isExpanded: $isExpanded) {
            ForEach(links.keys.sorted(), id: \.self) { link in
                HStack {
                    URLValue(link, isLink: true)
                    Spacer(minLength: 8)
                    if let count = links[link], count > 1 {
                        Text("\(count)×")
                            .monospacedDigit()
                            .foregroundStyle(.secondary)
                    }
                }
            }
        } label: {
            Text(links.isEmpty ? "No links" : "\(links.count) links")
        }
        .disabled(links.isEmpty)
    }
}

extension SemanticShapeVO.ScoreNode {
    /// summary shows the points of the maximum and the share in percent
    var summary: String {
        guard max > 0 else {
            return "\(points)/\(max)"
        }
        let share = Double(points) / Double(max)
        return "\(points)/\(max) (\(share.formatted(.percent.precision(.fractionLength(1)))))"
    }
}
