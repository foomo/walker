import SwiftUI

/// ResultLookup finds the result for a url the preview navigated to - nil, if
/// walker did not scrape it
typealias ResultLookup = @MainActor (String) async throws -> ScrapeResult?

struct ResultInspector: View {
    let result: ScrapeResult
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
    let start: ScrapeResult
    let lookup: ResultLookup
    /// highlight outlines links to this url in the preview
    var highlight: String?

    @State private var browsedURL: String?
    @State private var browsed = Browsed.start
    @State private var preview = PreviewController()

    enum Browsed {
        case start
        case loading(String)
        case found(ScrapeResult)
        case notScraped(String)
        case failed(String, String)
    }

    /// shown is the result the inspector shows - the start, or where the preview went
    private var shown: ScrapeResult? {
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

extension ScrapeResult {
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
    let result: ScrapeResult
    /// preview shows the page of the result - hovering a heading highlights it there
    var preview: PreviewController?

    var body: some View {
        Group {
            // right below the preview: the outline and the links of the page
            if let headings = result.structure.headings, !headings.isEmpty {
                Section("Headings") {
                    HeadingOutline(headings: headings) { position in
                        preview?.highlightHeading(position)
                    }
                }
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
                Section("Schema validation") {
                    LabeledContent("Score", value: report.score.formatted())
                    ForEach(Array((report.validations ?? []).compactMap { $0 }.enumerated()), id: \.offset) { _, validation in
                        LabeledContent(validation.type.rawValue) {
                            Text("\(validation.comment) (−\(validation.penalty))")
                        }
                    }
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

/// HeadingOutline renders the headings of a page as an indented outline -
/// sized by level - and marks what search engines dislike: empty headings,
/// skipped levels and more than one h1.
struct HeadingOutline: View {
    let headings: [Heading]
    /// onHover reports the heading under the mouse - nil, when it left
    var onHover: ((Position?) -> Void)?

    /// Position identifies a heading in the page: its level and text, which of
    /// the headings with that text it is, and which of its level
    struct Position {
        let level: Int
        let text: String
        let occurrence: Int
        let levelIndex: Int
    }

    private var positions: [Position] {
        var occurrences: [String: Int] = [:]
        var levelIndices: [Int: Int] = [:]
        return headings.map { heading in
            let key = "\(heading.level)|\(heading.text)"
            let position = Position(
                level: heading.level,
                text: heading.text,
                occurrence: occurrences[key, default: 0],
                levelIndex: levelIndices[heading.level, default: 0]
            )
            occurrences[key, default: 0] += 1
            levelIndices[heading.level, default: 0] += 1
            return position
        }
    }

    private static let indent: CGFloat = 14

    /// Issue of a heading in the outline
    enum Issue {
        case empty
        case skipped(Int)
        case anotherH1

        var help: String {
            switch self {
            case .empty: "empty heading"
            case let .skipped(level): "skips h\(level)"
            case .anotherH1: "more than one h1"
            }
        }
    }

    /// issues per heading index
    private var issues: [Int: Issue] {
        var issues: [Int: Issue] = [:]
        var previous = 0
        var hasH1 = false
        for (index, heading) in headings.enumerated() {
            if heading.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                issues[index] = .empty
            } else if heading.level == 1 && hasH1 {
                issues[index] = .anotherH1
            } else if heading.level > previous + 1 {
                issues[index] = .skipped(previous + 1)
            }
            hasH1 = hasH1 || heading.level == 1
            previous = heading.level
        }
        return issues
    }

    var body: some View {
        let issues = issues
        let positions = positions
        VStack(alignment: .leading, spacing: 6) {
            ForEach(Array(headings.enumerated()), id: \.offset) { index, heading in
                row(heading, issue: issues[index])
                    .contentShape(.rect)
                    .onHover { inside in
                        onHover?(inside ? positions[index] : nil)
                    }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 4)
    }

    private func row(_ heading: Heading, issue: Issue?) -> some View {
        let depth = max(min(heading.level, 6) - 1, 0)
        return HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text("H\(heading.level)")
                .font(.caption2.monospaced().weight(.semibold))
                .foregroundStyle(issue == nil ? Color.secondary : Color.orange)
                .padding(.horizontal, 4)
                .padding(.vertical, 1)
                .background(issue == nil ? Color.secondary.opacity(0.12) : Color.orange.opacity(0.15), in: .rect(cornerRadius: 3))
                .help(issue?.help ?? "")
            if case .empty = issue {
                Text("(empty)")
                    .italic()
                    .foregroundStyle(.orange)
            } else {
                Text(verbatim: heading.text.trimmingCharacters(in: .whitespacesAndNewlines))
                    .font(Self.font(for: heading.level))
                    .foregroundStyle(heading.level >= 5 ? .secondary : .primary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(.leading, CGFloat(depth) * Self.indent)
        .overlay(alignment: .leading) {
            // guide lines for the nesting
            HStack(spacing: Self.indent - 1) {
                ForEach(0..<depth, id: \.self) { _ in
                    Rectangle()
                        .fill(.quaternary)
                        .frame(width: 1)
                }
            }
            .padding(.leading, 4)
        }
    }

    private static func font(for level: Int) -> Font {
        switch level {
        case 1: .title3.weight(.bold)
        case 2: .headline
        case 3: .body.weight(.medium)
        case 4: .body
        default: .callout
        }
    }
}
