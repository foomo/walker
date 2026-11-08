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
/// live page, fetched like the crawler does - and shows it. The content of a
/// branch is loaded when the user clicks it.
struct SemanticOutlineSection: View {
    let url: String
    /// preview shows the page - hovering an element highlights it there
    var preview: PreviewController?

    @Environment(\.semanticDocumentLoader) private var loader
    @State private var state = LoadState.loading
    /// loads counts the loaded outlines - a new one starts without content
    @State private var loads = 0

    enum LoadState {
        case loading
        case loaded(WalkerVO.SemanticDocument)
        case failed(String)
    }

    var body: some View {
        Section {
            content
        } header: {
            Text("Document outline")
                .help("The landmarks, articles, sections, asides and headings of the page, as the server fetches it now - click an element to show the content of its branch")
        }
        .task(id: url) {
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
            SemanticDocumentContent(result: result, pageURL: URL(string: url), loadContent: loadContent) { selector, element in
                preview?.highlight(selector: selector, element: element)
            }
            .id(loads)
        }
    }

    private var loadContent: SemanticDocumentContent.ContentLoader? {
        guard let loader else { return nil }
        return { [url] in try await loader(url, true) }
    }

    private func load() async {
        guard let loader else {
            state = .failed("No server to load the outline from.")
            return
        }
        state = .loading
        do {
            let result = try await loader(url, false)
            guard !Task.isCancelled else { return }
            loads += 1
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
    /// ContentLoader loads the same document with its content - texts,
    /// links, images, figures, lists and tables
    typealias ContentLoader = @MainActor () async throws -> WalkerVO.SemanticDocument

    let result: WalkerVO.SemanticDocument
    /// pageURL resolves relative image sources
    var pageURL: URL?
    /// loadContent is called when the user opens the content of a branch the first time
    var loadContent: ContentLoader?
    /// marked are locators of page elements selected elsewhere - their rows are marked
    var marked: [String] = []
    /// selected is the selector of the element selected in the outline
    var selected: String?
    /// onSelect is called with an element the user clicked - nil: the outline has no selection
    var onSelect: ((String, SemanticScrapeVO.DocumentElement) -> Void)?
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
                SemanticOutline(elements: body, pageURL: pageURL, loadContent: loadBody, marked: marked, selected: selected, onSelect: onSelect, onHover: onHover)
            }
            if !document.lang.isEmpty {
                LabeledContent("Language", value: document.lang)
            }
        }
    }

    private var loadBody: SemanticOutline.ContentLoader? {
        guard let loadContent else { return nil }
        return {
            let result = try await loadContent()
            if !result.error.isEmpty {
                throw SemanticOutline.ContentError(message: result.error)
            }
            return result.document?.body ?? []
        }
    }
}

/// SemanticOutline renders the outline of a page as an indented tree:
/// landmarks and containers with their labels, the links of navs, headings
/// sized by level. Clicking an element shows the content of its branch - texts,
/// teaser links, images, figures, lists and tables, a table as a grid - and
/// clicking it again hides it; the content is loaded with the first click.
/// Containers collapse - navs start collapsed, menus are long. Headings are
/// marked for what search engines dislike: empty headings, skipped levels and
/// more than one h1.
struct SemanticOutline: View {
    /// ContentLoader loads the outline with its content - the same structure,
    /// the content elements between its elements
    typealias ContentLoader = @MainActor () async throws -> [SemanticScrapeVO.DocumentElement]

    struct ContentError: LocalizedError {
        let message: String
        var errorDescription: String? { message }
    }

    /// elements are the outline without content
    let elements: [SemanticScrapeVO.DocumentElement]
    /// pageURL resolves relative image sources
    var pageURL: URL?
    /// loadContent loads the content - without it, clicks show nothing
    var loadContent: ContentLoader?
    /// marked are locators of page elements selected elsewhere, e.g. a node of
    /// the shape tree - same scheme as the selectors of the outline
    var marked: [String] = []
    /// selected is the selector of the element selected in the outline
    var selected: String?
    /// onSelect is called with an element the user clicked - nil: the outline has no selection
    var onSelect: ((String, SemanticScrapeVO.DocumentElement) -> Void)?
    /// onHover reports the selector under the mouse, with its element - a link
    /// has none - and nil, when it left
    var onHover: ((String?, SemanticScrapeVO.DocumentElement?) -> Void)?

    /// containers the user opened or closed, by selector - against their default
    @State private var toggled: Set<String> = []
    /// branches whose content the user showed (true) or hid (false), by
    /// selector - an element without an entry follows its parent, the outline
    /// starts without content
    @State private var branches: [String: Bool] = [:]
    @State private var content = ContentState.none

    enum ContentState {
        case none
        case loading
        case loaded([SemanticScrapeVO.DocumentElement])
        case failed(String)
    }

    /// source is the tree the rows come from: the outline with content once it is loaded
    private var source: [SemanticScrapeVO.DocumentElement] {
        if case let .loaded(elements) = content {
            return elements
        }
        return elements
    }

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
        /// showsContent: the content of this row's branch is shown
        var showsContent = false

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
    /// collapsed containers, and content only in the branches that show it
    private var rows: [Row] {
        var rows: [Row] = []
        func visit(_ elements: [SemanticScrapeVO.DocumentElement], parent: SemanticScrapeVO.ElementType?, path: String, depth: Int, showsContent inherited: Bool) {
            for (index, element) in elements.enumerated() {
                let id = "\(path)/\(index)"
                if element.isContent, !inherited {
                    // hidden content - an outline element in it, which the content pass never makes, stays
                    visit(element.children ?? [], parent: element.elementType, path: id, depth: depth, showsContent: false)
                    continue
                }
                let showsContent = element.isContent ? inherited : branches[element.selector] ?? inherited
                let marker: String? = switch parent {
                case .unorderedList: "•"
                case .orderedList: "\(index + 1)."
                default: nil
                }
                rows.append(Row(id: id, content: .element(element), depth: depth, marker: marker, showsContent: showsContent))
                if isCollapsed(element) {
                    continue
                }
                // the links of a nav are structure, all others - teasers of headings, links in texts - content
                if element.elementType == .nav || showsContent {
                    for (linkIndex, link) in (element.links ?? []).enumerated() {
                        rows.append(Row(id: "\(id)/link\(linkIndex)", content: .link(link), depth: depth + 1, showsContent: showsContent))
                    }
                }
                if element.elementType == .table {
                    if !(element.children ?? []).isEmpty {
                        rows.append(Row(id: "\(id)/grid", content: .grid(element), depth: depth + 1, showsContent: showsContent))
                    }
                } else {
                    visit(element.children ?? [], parent: element.elementType, path: id, depth: depth + 1, showsContent: showsContent)
                }
            }
        }
        visit(source, parent: nil, path: "", depth: 0, showsContent: false)
        return rows
    }

    /// toggleContent shows or hides the content of the branch of an element -
    /// its descendants follow it - and loads the content the first time
    private func toggleContent(_ element: SemanticScrapeVO.DocumentElement, showsContent: Bool) {
        func forget(_ elements: [SemanticScrapeVO.DocumentElement]) {
            for child in elements {
                branches[child.selector] = nil
                forget(child.children ?? [])
            }
        }
        forget(element.children ?? [])
        branches[element.selector] = !showsContent
        if !showsContent {
            // a collapsed container opens to show its content
            if isCollapsed(element) {
                toggled.formSymmetricDifference([element.selector])
            }
            Task { await loadContentIfNeeded() }
        }
    }

    private func loadContentIfNeeded() async {
        switch content {
        case .loading, .loaded: return
        case .none, .failed: break
        }
        guard let loadContent else { return }
        content = .loading
        do {
            content = .loaded(try await loadContent())
        } catch is CancellationError {
            content = .none
        } catch {
            content = .failed(error.localizedDescription)
        }
    }

    /// Mark is how a row relates to the marked page elements
    enum Mark {
        /// the row is a marked element - or the outline element directly in a marked one
        case element
        /// a marked element is inside the row, e.g. an image in a section or a link in a collapsed nav
        case contains
    }

    /// marks are the marks of the visible element rows by id: a row with the
    /// locator itself, else the outermost rows inside it, else the nearest row
    /// around it. Content rows share their container's selector and get none.
    private func marks(_ rows: [Row]) -> [String: Mark] {
        guard !marked.isEmpty else { return [:] }
        let elementRows = rows.filter {
            if case let .element(element) = $0.content { return !element.isContent }
            return false
        }
        var marks: [String: Mark] = [:]
        for locator in marked {
            let exact = elementRows.filter { $0.selector == locator }
            if !exact.isEmpty {
                exact.forEach { marks[$0.id] = .element }
                continue
            }
            let inside = elementRows.filter { $0.selector.hasPrefix(locator + " > ") }
            let outermost = inside.filter { row in
                !inside.contains { other in other.id != row.id && row.selector.hasPrefix(other.selector + " > ") }
            }
            if !outermost.isEmpty {
                outermost.forEach { marks[$0.id] = .element }
                continue
            }
            if let around = elementRows.filter({ locator.hasPrefix($0.selector + " > ") }).max(by: { $0.selector.count < $1.selector.count }),
               marks[around.id] == nil {
                marks[around.id] = .contains
            }
        }
        return marks
    }

    var body: some View {
        let issues = issues
        let rows = rows
        let marks = marks(rows)
        ScrollViewReader { proxy in
            outline(rows: rows, issues: issues, marks: marks)
                .onChange(of: marked) {
                    // the first marked row, in document order
                    if let first = rows.first(where: { marks[$0.id] != nil }) {
                        withAnimation {
                            proxy.scrollTo(first.id, anchor: .center)
                        }
                    }
                }
        }
    }

    private func outline(rows: [Row], issues: [String: Issue], marks: [String: Mark]) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            if case let .failed(message) = content {
                Label("The content could not be loaded: \(message)", systemImage: "exclamationmark.triangle")
                    .font(.callout)
                    .foregroundStyle(.orange)
            }
            ForEach(rows, id: \.id) { row in
                content(row, issues: issues)
                    .padding(.leading, CGFloat(row.depth) * Self.indent)
                    .background(alignment: .leading) {
                        if isSelected(row) {
                            selectionBackground(depth: row.depth)
                        } else {
                            markBackground(marks[row.id], depth: row.depth)
                        }
                    }
                    .id(row.id)
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
            } else if element.isContent || loadContent == nil {
                self.row(element, issue: issues[element.selector])
            } else {
                HStack(alignment: .firstTextBaseline, spacing: 6) {
                    self.row(element, issue: issues[element.selector])
                    Spacer(minLength: 4)
                    contentIndicator(row.showsContent)
                }
                // selectable text - the outline panes enable it - would take the click
                .textSelection(.disabled)
                .contentShape(.rect)
                .onTapGesture {
                    toggleContent(element, showsContent: row.showsContent)
                    onSelect?(element.selector, element)
                }
                .pointerStyle(.link)
                .help(onSelect == nil
                    ? (row.showsContent ? "Click to hide the content of this branch" : "Click to show the texts, links, images, lists and tables of this branch")
                    : (row.showsContent ? "Click to select it and hide the content of this branch" : "Click to select it and show the texts, links, images, lists and tables of this branch"))
            }
        case let .link(link):
            linkRow(link)
        case let .grid(table):
            tableGrid(table)
        }
    }

    /// isSelected: the row is the element selected in the outline - its
    /// content rows share the selector and are not
    private func isSelected(_ row: Row) -> Bool {
        guard onSelect != nil, let selected, case let .element(element) = row.content else { return false }
        return !element.isContent && element.selector == selected
    }

    /// selectionColor is the color the outline highlights in the page with
    static let selectionColor = Color(red: 0xaf / 255, green: 0x52 / 255, blue: 0xde / 255)

    /// selectionBackground marks the selected row - in the color of its highlight in the page
    private func selectionBackground(depth: Int) -> some View {
        RoundedRectangle(cornerRadius: 4)
            .fill(Self.selectionColor.opacity(0.22))
            .overlay(alignment: .leading) {
                Capsule()
                    .fill(Self.selectionColor)
                    .frame(width: 3)
                    .padding(.vertical, 2)
            }
            .padding(.leading, CGFloat(depth) * Self.indent - 4)
            .padding(.vertical, -2)
    }

    /// markBackground tints a marked row from its indentation on - an element
    /// stronger than a row that contains one
    @ViewBuilder
    private func markBackground(_ mark: Mark?, depth: Int) -> some View {
        if let mark {
            RoundedRectangle(cornerRadius: 4)
                .fill(Color.accentColor.opacity(mark == .element ? 0.2 : 0.08))
                .overlay(alignment: .leading) {
                    if mark == .element {
                        Capsule()
                            .fill(Color.accentColor)
                            .frame(width: 3)
                            .padding(.vertical, 2)
                    }
                }
                .padding(.leading, CGFloat(depth) * Self.indent - 4)
                .padding(.vertical, -2)
                .help(mark == .element ? "selected in the shape tree" : "contains an element selected in the shape tree")
        }
    }

    /// contentIndicator marks a branch that shows its content - a spinner
    /// while the content loads
    @ViewBuilder
    private func contentIndicator(_ showsContent: Bool) -> some View {
        if showsContent {
            if case .loading = content {
                ProgressView()
                    .controlSize(.mini)
            } else {
                Image(systemName: "text.alignleft")
                    .font(.caption2)
                    .foregroundStyle(Color.accentColor)
            }
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
    /// isContent: texts, images, figures, lists and tables - what the outline
    /// gets with content, between its elements
    var isContent: Bool {
        [
            .text, .image, .figure,
            .unorderedList, .orderedList, .listItem, .definitions, .term, .definition,
            .table, .row, .headerCell, .cell,
        ].contains(elementType)
    }

    /// outlineLabel names the element as the outline shows it: its type and
    /// its label, or its text
    var outlineLabel: String {
        let name = label.isEmpty ? text : label
        return name.isEmpty ? elementType.rawValue : "\(elementType.rawValue) \(name)"
    }

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
