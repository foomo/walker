import SwiftUI

/// SemanticDocumentLoader has the server extract the semantic outline of a
/// page - with or without its content
typealias SemanticDocumentLoader = @MainActor @Sendable (_ url: String, _ content: Bool) async throws -> WalkerVO.SemanticDocument

/// SemanticDocumentLoaderKey hands the loader of the server down to the inspectors
private struct SemanticDocumentLoaderKey: EnvironmentKey {
    static let defaultValue: SemanticDocumentLoader? = nil
}

extension EnvironmentValues {
    var semanticDocumentLoader: SemanticDocumentLoader? {
        get { self[SemanticDocumentLoaderKey.self] }
        set { self[SemanticDocumentLoaderKey.self] = newValue }
    }
}

/// SemanticOutlineSection loads the outline of a page from the server - the
/// live page, fetched like the crawler does - and shows it
struct SemanticOutlineSection: View {
    let url: String
    /// preview shows the page - hovering an element highlights it there
    var preview: PreviewController?

    @Environment(\.semanticDocumentLoader) private var loader
    @State private var state = LoadState.loading
    /// showsContent adds texts, links, images and figures to the outline
    @AppStorage("outlineShowsContent") private var showsContent = false

    enum LoadState {
        case loading
        case loaded(WalkerVO.SemanticDocument)
        case failed(String)
    }

    var body: some View {
        Section {
            content
        } header: {
            HStack {
                Text("Document outline")
                    .help("The landmarks, articles, sections, asides and headings of the page, as the server fetches it now")
                Spacer()
                Toggle("Content", isOn: $showsContent)
                    .toggleStyle(.switch)
                    .controlSize(.mini)
                    .help(showsContent ? "Show the structure only" : "Show the texts, links, images and figures too")
            }
        }
        .task(id: "\(showsContent) \(url)") {
            await load()
        }
    }

    @ViewBuilder
    private var content: some View {
        switch state {
        case .loading:
            ProgressView()
                .controlSize(.small)
                .frame(maxWidth: .infinity)
        case let .failed(message):
            Label(message, systemImage: "exclamationmark.triangle")
                .foregroundStyle(.red)
        case let .loaded(result):
            SemanticDocumentContent(result: result, pageURL: URL(string: url)) { selector, element in
                preview?.highlight(selector: selector, element: element)
            }
        }
    }

    private func load() async {
        guard let loader else {
            state = .failed("No server to load the outline from.")
            return
        }
        state = .loading
        do {
            let result = try await loader(url, showsContent)
            guard !Task.isCancelled else { return }
            state = .loaded(result)
        } catch is CancellationError {
            return
        } catch {
            state = .failed(error.localizedDescription)
        }
    }
}

/// SemanticDocumentContent shows a loaded semantic document: its error, a
/// missing main, the outline and the language - rows of a form section or a
/// stack
struct SemanticDocumentContent: View {
    let result: WalkerVO.SemanticDocument
    /// pageURL resolves relative image sources
    var pageURL: URL?
    var onHover: ((String?, SemanticScrapeVO.DocumentElement?) -> Void)?

    var body: some View {
        if !result.error.isEmpty {
            Label(result.error, systemImage: "exclamationmark.triangle")
                .foregroundStyle(.red)
        } else if let document = result.document {
            let body = document.body ?? []
            if !body.contains(where: \.containsMain) {
                Label("The page has no main.", systemImage: "rectangle.dashed")
                    .foregroundStyle(.orange)
            }
            if body.isEmpty {
                Label("The page has no landmarks, articles, sections, asides or headings.", systemImage: "rectangle.dashed")
                    .foregroundStyle(.orange)
            } else {
                SemanticOutline(elements: body, pageURL: pageURL, onHover: onHover)
            }
            if !document.lang.isEmpty {
                LabeledContent("Language", value: document.lang)
            }
        }
    }
}

/// SemanticOutline renders the outline of a page as an indented tree:
/// landmarks and containers with their labels, the links of navs, headings
/// sized by level, and with content texts, images, figures, lists and tables -
/// a table as a grid. Containers collapse - navs start collapsed, menus are long.
/// Headings are marked for what search engines dislike: empty headings,
/// skipped levels and more than one h1.
struct SemanticOutline: View {
    let elements: [SemanticScrapeVO.DocumentElement]
    /// pageURL resolves relative image sources
    var pageURL: URL?
    /// onHover reports the selector under the mouse, with its element - a link
    /// has none - and nil, when it left
    var onHover: ((String?, SemanticScrapeVO.DocumentElement?) -> Void)?

    /// containers the user opened or closed, by selector - against their default
    @State private var toggled: Set<String> = []

    private static let indent: CGFloat = 14

    /// Row is an element or a link of the tree at its depth
    struct Row {
        /// id is the path of the row in the tree - selectors are not unique,
        /// a text in a container has the container's
        let id: String
        enum Content {
            case element(SemanticScrapeVO.DocumentElement)
            case link(SemanticScrapeVO.Link)
            /// the rows and cells of a table
            case grid(SemanticScrapeVO.DocumentElement)
        }

        let content: Content
        let depth: Int
        /// marker of a list item: a bullet or its number
        var marker: String?

        var selector: String {
            switch content {
            case let .element(element), let .grid(element): element.selector
            case let .link(link): link.selector
            }
        }
    }

    private func isCollapsed(_ element: SemanticScrapeVO.DocumentElement) -> Bool {
        (element.elementType == .nav) != toggled.contains(element.selector)
    }

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

    /// issues per heading selector, in document order - collapsed or not
    private var issues: [String: Issue] {
        var issues: [String: Issue] = [:]
        var previous = 0
        var hasH1 = false
        func visit(_ elements: [SemanticScrapeVO.DocumentElement]) {
            for element in elements {
                if let level = element.headingLevel {
                    if element.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        issues[element.selector] = .empty
                    } else if level == 1 && hasH1 {
                        issues[element.selector] = .anotherH1
                    } else if level > previous + 1 {
                        issues[element.selector] = .skipped(previous + 1)
                    }
                    hasH1 = hasH1 || level == 1
                    previous = level
                }
                visit(element.children ?? [])
            }
        }
        visit(elements)
        return issues
    }

    /// rows are the visible elements and links - without the content of
    /// collapsed containers
    private var rows: [Row] {
        var rows: [Row] = []
        func visit(_ elements: [SemanticScrapeVO.DocumentElement], parent: SemanticScrapeVO.ElementType?, path: String, depth: Int) {
            for (index, element) in elements.enumerated() {
                let id = "\(path)/\(index)"
                let marker: String? = switch parent {
                case .unorderedList: "•"
                case .orderedList: "\(index + 1)."
                default: nil
                }
                rows.append(Row(id: id, content: .element(element), depth: depth, marker: marker))
                if isCollapsed(element) {
                    continue
                }
                for (linkIndex, link) in (element.links ?? []).enumerated() {
                    rows.append(Row(id: "\(id)/link\(linkIndex)", content: .link(link), depth: depth + 1))
                }
                if element.elementType == .table {
                    if !(element.children ?? []).isEmpty {
                        rows.append(Row(id: "\(id)/grid", content: .grid(element), depth: depth + 1))
                    }
                } else {
                    visit(element.children ?? [], parent: element.elementType, path: id, depth: depth + 1)
                }
            }
        }
        visit(elements, parent: nil, path: "", depth: 0)
        return rows
    }

    var body: some View {
        let issues = issues
        VStack(alignment: .leading, spacing: 6) {
            ForEach(rows, id: \.id) { row in
                content(row, issues: issues)
                    .padding(.leading, CGFloat(row.depth) * Self.indent)
                    .overlay(alignment: .leading) {
                        guides(depth: row.depth)
                    }
                    .contentShape(.rect)
                    .onHover { inside in
                        if !inside {
                            onHover?(nil, nil)
                        } else if case let .element(element) = row.content {
                            onHover?(element.selector, element)
                        } else {
                            onHover?(row.selector, nil)
                        }
                    }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 4)
    }

    @ViewBuilder
    private func content(_ row: Row, issues: [String: Issue]) -> some View {
        switch row.content {
        case let .element(element):
            if element.isListItem {
                listItemRow(element, marker: row.marker)
            } else {
                self.row(element, issue: issues[element.selector])
            }
        case let .link(link):
            linkRow(link)
        case let .grid(table):
            tableGrid(table)
        }
    }

    /// guides are the lines of the nesting
    private func guides(depth: Int) -> some View {
        HStack(spacing: Self.indent - 1) {
            ForEach(0..<depth, id: \.self) { _ in
                Rectangle()
                    .fill(.quaternary)
                    .frame(width: 1)
            }
        }
        .padding(.leading, 4)
    }

    private func linkRow(_ link: SemanticScrapeVO.Link) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Image(systemName: "link")
                .font(.caption2)
                .foregroundStyle(.tertiary)
                .frame(width: 10)
            if link.text.isEmpty {
                Text("(no name)")
                    .italic()
                    .foregroundStyle(.orange)
                    .help("the link has no accessible name")
            } else {
                Text(verbatim: link.text)
                    .font(.callout)
                    .lineLimit(1)
            }
            Spacer(minLength: 8)
            Text(verbatim: link.href)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.middle)
                .help(link.href)
        }
    }

    private func row(_ element: SemanticScrapeVO.DocumentElement, issue: Issue?) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            if element.elementType == .text {
                Image(systemName: "text.alignleft")
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
                    .frame(width: 10)
                Text(verbatim: element.text)
                    .font(.callout)
                    .lineLimit(4)
                    .fixedSize(horizontal: false, vertical: true)
                    .help(element.text)
            } else if element.elementType == .image {
                imageRow(element)
            } else if let level = element.headingLevel {
                chip(element.elementType.rawValue.uppercased(), style: issue == nil ? .plain : .issue)
                    .help(issue?.help ?? "")
                if case .empty = issue {
                    Text("(empty)")
                        .italic()
                        .foregroundStyle(.orange)
                } else {
                    Text(verbatim: element.text)
                        .font(Self.font(for: level))
                        .foregroundStyle(level >= 5 ? .secondary : .primary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            } else {
                disclosure(element)
                chip(element.elementType.rawValue, style: element.isLandmark ? .landmark : .plain)
                if [.figure, .table].contains(element.elementType), !element.text.isEmpty {
                    // the caption
                    Text(verbatim: element.text)
                        .font(.callout)
                        .lineLimit(2)
                        .help(element.text)
                }
                if !element.label.isEmpty {
                    Text(verbatim: element.label)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.tail)
                        .help(element.label)
                }
                if let links = element.links, !links.isEmpty, isCollapsed(element) {
                    Text("\(links.count) links")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }
            }
        }
    }

    /// listItemRow shows the marker and the text of a list item - a term bold,
    /// a definition secondary
    private func listItemRow(_ item: SemanticScrapeVO.DocumentElement, marker: String?) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Text(verbatim: marker ?? "")
                .font(.callout.monospacedDigit())
                .foregroundStyle(.secondary)
                .frame(minWidth: 10, alignment: .trailing)
            if !item.text.isEmpty {
                Text(verbatim: item.text)
                    .font(item.elementType == .term ? .callout.weight(.semibold) : .callout)
                    .foregroundStyle(item.elementType == .definition ? .secondary : .primary)
                    .lineLimit(4)
                    .fixedSize(horizontal: false, vertical: true)
                    .help(item.text)
            }
        }
    }

    /// tableGrid shows the rows and cells of a table - header cells bold,
    /// scrolling sideways when the table is wider than the column
    private func tableGrid(_ table: SemanticScrapeVO.DocumentElement) -> some View {
        ScrollView(.horizontal) {
            Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 4) {
                ForEach(Array((table.children ?? []).enumerated()), id: \.offset) { index, row in
                    if index > 0 {
                        Divider()
                    }
                    GridRow(alignment: .firstTextBaseline) {
                        ForEach(Array((row.children ?? []).enumerated()), id: \.offset) { _, cell in
                            Text(verbatim: cell.text)
                                .font(cell.elementType == .headerCell ? .caption.weight(.semibold) : .caption)
                                .foregroundStyle(cell.links?.isEmpty == false ? Color.accentColor : .primary)
                                .lineLimit(3)
                                .frame(maxWidth: 220, alignment: .leading)
                                .help(cell.links?.map(\.href).joined(separator: "\n") ?? "")
                                .onHover { inside in
                                    onHover?(inside ? cell.selector : nil, inside ? cell : nil)
                                }
                        }
                    }
                }
            }
            .padding(6)
        }
        .background(.quaternary.opacity(0.3), in: .rect(cornerRadius: 4))
    }

    /// imageRow shows a thumbnail, the alt text and the source of an image
    private func imageRow(_ image: SemanticScrapeVO.DocumentElement) -> some View {
        HStack(alignment: .center, spacing: 6) {
            AsyncImage(url: URL(string: image.src, relativeTo: pageURL)) { phase in
                if let loaded = phase.image {
                    loaded
                        .resizable()
                        .scaledToFill()
                } else {
                    Image(systemName: phase.error == nil ? "photo" : "exclamationmark.triangle")
                        .foregroundStyle(.tertiary)
                }
            }
            .frame(width: 40, height: 28)
            .clipShape(.rect(cornerRadius: 3))
            .overlay(RoundedRectangle(cornerRadius: 3).strokeBorder(.quaternary))
            if image.text.isEmpty {
                Text("no alt text")
                    .italic()
                    .foregroundStyle(.secondary)
                    .help("decorative, or the alt text is missing")
            } else {
                Text(verbatim: image.text)
                    .font(.callout)
                    .lineLimit(2)
            }
            Spacer(minLength: 8)
            Text(verbatim: image.src)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.middle)
                .help(image.src)
        }
    }

    /// disclosure collapses a container with content, the rest gets the same width
    @ViewBuilder
    private func disclosure(_ element: SemanticScrapeVO.DocumentElement) -> some View {
        let isCollapsed = isCollapsed(element)
        let count = (element.children?.count ?? 0) + (element.links?.count ?? 0)
        if count > 0 {
            Button {
                if toggled.contains(element.selector) {
                    toggled.remove(element.selector)
                } else {
                    toggled.insert(element.selector)
                }
            } label: {
                Image(systemName: "chevron.right")
                    .rotationEffect(.degrees(isCollapsed ? 0 : 90))
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(.secondary)
                    .frame(width: 10)
            }
            .buttonStyle(.plain)
            .help(isCollapsed ? "Show the content" : "Hide the content")
        } else {
            Color.clear.frame(width: 10, height: 1)
        }
    }

    enum ChipStyle {
        case plain, landmark, issue

        var color: Color {
            switch self {
            case .plain: .secondary
            case .landmark: .accentColor
            case .issue: .orange
            }
        }
    }

    private func chip(_ text: String, style: ChipStyle) -> some View {
        Text(text)
            .font(.caption2.monospaced().weight(.semibold))
            .foregroundStyle(style.color)
            .padding(.horizontal, 4)
            .padding(.vertical, 1)
            .background(style.color.opacity(style == .plain ? 0.12 : 0.15), in: .rect(cornerRadius: 3))
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

extension SemanticScrapeVO.DocumentElement {
    /// isListItem: li, dt and dd
    var isListItem: Bool {
        [.listItem, .term, .definition].contains(elementType)
    }

    /// isLandmark: the page regions main, header, nav and footer
    var isLandmark: Bool {
        [.main, .header, .nav, .footer].contains(elementType)
    }

    /// containsMain reports the main in this element or below it
    var containsMain: Bool {
        elementType == .main || (children ?? []).contains(where: \.containsMain)
    }

    /// headingLevel is 1 to 6 for headings, nil for containers
    var headingLevel: Int? {
        let type = elementType.rawValue
        guard type.count == 2, type.first == "h", let level = Int(type.dropFirst()) else { return nil }
        return level
    }
}
