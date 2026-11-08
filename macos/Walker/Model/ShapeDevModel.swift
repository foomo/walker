import Foundation
import Observation
import SwiftUI

/// ShapeDevModel is the state of a shape developer window: a page, a shape
/// root and a group, validated on the walker server. Client and server share
/// a file system - the shape root is a path on the server.
@MainActor
@Observable
final class ShapeDevModel {
    /// Source is the html the server validates
    enum Source: String, CaseIterable, Identifiable {
        /// the server fetches the page itself, like the crawler
        case server = "Server"
        /// a snapshot of the DOM in the web view
        case rendered = "Rendered"

        var id: String { rawValue }
    }

    let server: Server
    /// address is the text of the address field
    var address: String
    /// pageURL is the url loaded in the web view
    private(set) var pageURL: URL?
    /// reloadCount changes to reload the page in the web view
    private(set) var reloadCount = 0
    var shapeRoot: String {
        didSet { save() }
    }
    /// group to validate with - "" is the group the page reports, then default.semanticshape.html
    var group: String {
        didSet { save() }
    }
    /// groupHeader names the response header the group of a page is read
    /// from - "" is the one of the server's config. A local server takes it
    /// from its config, which may be newer than the running walker's.
    var groupHeader: String {
        didSet { save() }
    }

    /// effectiveGroupHeader is the header the validation reads the group from
    var effectiveGroupHeader: String {
        groupHeader.isEmpty ? rootInfo?.groupHeader ?? "" : groupHeader
    }
    var source: Source {
        didSet { save() }
    }
    private(set) var recentRoots: [String]

    private(set) var rootInfo: WalkerVO.ShapeRootInfo?
    private(set) var validation: WalkerVO.ShapeValidation?
    /// items is the trace of the validation as a tree for the outline
    private(set) var items: [TraceItem] = []
    /// validatedLocators are the page nodes the shape matched
    private(set) var validatedLocators: Set<String> = []
    private(set) var isValidating = false
    private(set) var error: String?
    /// shapeReloadedAt is when a change of the shape files was picked up last
    private(set) var shapeReloadedAt: Date?
    /// highlighted is how many locators of the selection were found in the page - nil without selection
    private(set) var highlighted: (found: Int, total: Int)?
    /// selectedHTML are the elements the selected shape node matched, as html
    /// from the page in the web view - nil without selection
    private(set) var selectedHTML: [ShapeHighlighter.ElementHTML]?
    /// pageVersion counts the pages the web view loaded - the outline reloads with it
    private(set) var pageVersion = 0
    /// outline is the semantic document of the page, from the same source as the validation
    private(set) var outline: OutlineState = .idle
    /// outlineVersion counts the loaded outlines - a new one starts without content
    private(set) var outlineVersion = 0

    enum OutlineState {
        case idle
        case loading
        case loaded(WalkerVO.SemanticDocument)
        case failed(String)
    }

    /// configRoot is the shape root of the config of a local server, nil if it has none
    private(set) var configRoot: String?

    /// selection is the id of the selected trace item
    var selection: TraceItem.ID? {
        didSet {
            // one selection at a time - a shape node ends the one in the outline
            if selection != nil {
                outlineSelection = nil
            }
            highlightSelection()
        }
    }

    /// OutlineSelection is an element selected in the document outline
    struct OutlineSelection: Equatable {
        let selector: String
        /// label names it in the html pane, e.g. section Feeding
        let label: String
    }

    /// outlineSelection is the element selected in the outline - instead of a
    /// shape node it is highlighted in the page and shown in the html pane
    private(set) var outlineSelection: OutlineSelection?

    /// selectOutline selects an element of the outline - the shape node is deselected
    func selectOutline(_ selector: String, label: String) {
        let outline = OutlineSelection(selector: selector, label: label)
        guard outline != outlineSelection else { return }
        if selection != nil {
            selection = nil
        }
        outlineSelection = outline
        highlightSelection()
    }
    /// collapsed trace items - nil until the first trace collapsed the clean subtrees
    var collapsed: Set<TraceItem.ID>?

    @ObservationIgnored let highlighter = ShapeHighlighter()
    @ObservationIgnored private let client: Walker.ServiceClient
    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private var itemsByID: [TraceItem.ID: TraceItem] = [:]
    @ObservationIgnored private var parentByID: [TraceItem.ID: TraceItem.ID] = [:]
    /// validationCount identifies the latest validation - older ones that finish later are dropped
    @ObservationIgnored private var validationCount = 0

    private static let recentRootsKey = "shapeDevRecentRoots"

    init(server: Server, defaults: UserDefaults = .standard) {
        self.server = server
        self.defaults = defaults
        client = Walker.ServiceClient(transport: URLSessionTransport(baseURL: server.baseURL))
        let prefix = "shapeDev.\(server.id.uuidString)."
        address = defaults.string(forKey: prefix + "address") ?? ""
        shapeRoot = defaults.string(forKey: prefix + "shapeRoot") ?? ""
        group = defaults.string(forKey: prefix + "group") ?? ""
        groupHeader = defaults.string(forKey: prefix + "groupHeader") ?? ""
        source = Source(rawValue: defaults.string(forKey: prefix + "source") ?? "") ?? .server
        recentRoots = defaults.stringArray(forKey: Self.recentRootsKey) ?? []
    }

    private func save() {
        let prefix = "shapeDev.\(server.id.uuidString)."
        defaults.set(address, forKey: prefix + "address")
        defaults.set(shapeRoot, forKey: prefix + "shapeRoot")
        defaults.set(group, forKey: prefix + "group")
        defaults.set(groupHeader, forKey: prefix + "groupHeader")
        defaults.set(source.rawValue, forKey: prefix + "source")
    }

    /// open loads the address in the web view - the validation follows when the page has loaded
    func open() {
        var text = address.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty else { return }
        if !text.contains("://") {
            text = "https://" + text
        }
        guard let url = URL(string: text) else {
            error = "invalid url \(text)"
            return
        }
        address = url.absoluteString
        save()
        if url == pageURL {
            reloadCount += 1
        } else {
            pageURL = url
        }
    }

    /// reload reloads the page - pageDidLoad reloads the shape root and validates
    func reload() {
        guard pageURL != nil else {
            open()
            return
        }
        reloadCount += 1
    }

    /// chooseRoot sets the shape root and remembers it
    func chooseRoot(_ path: String) {
        shapeRoot = path
        recentRoots.removeAll { $0 == path }
        recentRoots.insert(path, at: 0)
        recentRoots = Array(recentRoots.prefix(8))
        defaults.set(recentRoots, forKey: Self.recentRootsKey)
    }

    /// load takes the site and the shape root of a walker config: the page
    /// is the base url with the first start path, a relative shape root is
    /// resolved against the directory of the config, like walker does
    /// follow takes the shape root of the config of a local server - the
    /// section belongs to it - and its site as the first page
    func follow(_ form: ConfigTool.Form, configURL: URL) {
        if !form.shapeRoot.isEmpty {
            let root = form.shapeRoot.hasPrefix("/")
                ? URL(filePath: form.shapeRoot)
                : configURL.deletingLastPathComponent().appending(path: form.shapeRoot)
            let path = root.standardizedFileURL.path(percentEncoded: false)
            if path != shapeRoot {
                chooseRoot(path)
            }
            configRoot = path
        } else {
            configRoot = nil
        }
        if groupHeader != form.groupHeader {
            groupHeader = form.groupHeader
        }
        guard address.isEmpty, !form.baseURL.isEmpty else { return }
        var page = form.baseURL
        if let path = form.paths.first, !path.isEmpty, path != "/" {
            page = page.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + (path.hasPrefix("/") ? path : "/" + path)
        }
        address = page
    }

    func loadRoot() async {
        guard !shapeRoot.isEmpty else {
            rootInfo = nil
            return
        }
        do {
            let info = try await client.getShapeRoot(shapeRoot: shapeRoot)
            guard !Task.isCancelled else { return }
            rootInfo = info
            if !group.isEmpty, !(info.groups ?? []).contains(group) {
                group = ""
            }
        } catch is CancellationError {
            return
        } catch {
            rootInfo = nil
            self.error = error.localizedDescription
        }
    }

    /// pageDidLoad is called by the web view whenever a page finished loading
    /// watchShape polls the fingerprint of the shape files on the server and
    /// validates again when one is edited, added or removed - it runs until
    /// the task is cancelled, e.g. by a new shape root
    func watchShape() async {
        var version: String?
        while !Task.isCancelled {
            if !shapeRoot.isEmpty,
               let current = try? await client.getShapeRootVersion(shapeRoot: shapeRoot),
               !current.isEmpty {
                if let version, version != current {
                    shapeReloadedAt = .now
                    await loadRoot()
                    await validate()
                }
                version = current
            }
            try? await Task.sleep(for: .seconds(1))
        }
    }

    /// pageDidLoad is called by the web view whenever it shows another page -
    /// loaded or navigated to in a single page app - and refreshes like a reload
    func pageDidLoad(_ url: URL) async {
        address = url.absoluteString
        save()
        pageVersion += 1
        await loadRoot()
        await validate()
    }

    /// validate validates the page in the web view with the current settings
    func validate() async {
        guard let url = highlighter.pageURL ?? pageURL else { return }
        guard !shapeRoot.isEmpty else {
            error = "choose a shape root"
            return
        }
        // a page that is still loading has no DOM worth a snapshot - pageDidLoad validates it
        if source == .rendered, !highlighter.isPageLoaded {
            return
        }
        validationCount += 1
        let count = validationCount
        let requestedSource = source
        let requestedGroup = group
        let requestedHeader = groupHeader
        let requestedRoot = shapeRoot
        isValidating = true
        defer {
            if count == validationCount {
                isValidating = false
            }
        }
        do {
            var html = ""
            var resolvedGroup = requestedGroup
            if requestedSource == .rendered {
                if requestedGroup.isEmpty, !effectiveGroupHeader.isEmpty {
                    resolvedGroup = try await highlighter.groupHeaderValue(effectiveGroupHeader)
                }
                guard !Task.isCancelled, count == validationCount, highlighter.pageURL == url else { return }
                guard let snapshot = await highlighter.snapshot() else {
                    error = "the page has no DOM to validate yet"
                    return
                }
                html = snapshot
            }
            var validation = try await client.validateShape(shapeRoot: requestedRoot, group: resolvedGroup, groupHeader: requestedHeader, targetURL: url.absoluteString, html: html)
            if requestedSource == .rendered, requestedGroup.isEmpty {
                validation.detectedGroup = resolvedGroup
            }
            guard !Task.isCancelled, count == validationCount else { return }
            guard (highlighter.pageURL ?? pageURL) == url else { return }
            apply(validation)
            error = nil
        } catch is CancellationError {
            return
        } catch {
            guard !Task.isCancelled, count == validationCount, (highlighter.pageURL ?? pageURL) == url else { return }
            self.error = error.localizedDescription
        }
    }

    private func apply(_ validation: WalkerVO.ShapeValidation) {
        self.validation = validation
        items = validation.trace.map { [TraceItem(node: $0, parentID: "")] } ?? []
        itemsByID = [:]
        parentByID = [:]
        var clean: Set<TraceItem.ID> = []
        var validated: Set<String> = []
        func index(_ item: TraceItem, depth: Int) {
            itemsByID[item.id] = item
            if item.node.found, item.node.kind != "page" {
                validated.formUnion(item.node.locators ?? [])
            }
            for child in item.children ?? [] {
                parentByID[child.id] = item.id
            }
            // the page, html, head and body stay open, clean subtrees below are folded
            if depth > 2, item.children != nil, item.worst.isClean {
                clean.insert(item.id)
            }
            for child in item.children ?? [] {
                index(child, depth: depth + 1)
            }
        }
        items.forEach { index($0, depth: 0) }
        validatedLocators = validated
        if collapsed == nil {
            collapsed = clean
        }
        if let selection, itemsByID[selection] == nil {
            self.selection = nil
        }
        highlightSelection()
    }

    func item(_ id: TraceItem.ID?) -> TraceItem? {
        id.flatMap { itemsByID[$0] }
    }

    /// selectedLocators are the page nodes the selected trace node matched
    var selectedLocators: [String] {
        guard let node = item(selection)?.node, node.found else { return [] }
        return node.locators ?? []
    }

    /// headMark marks the selected shape node among the head elements: the
    /// rows it matched, the attributes it checks, or what is missing
    var headMark: HeadMark? {
        guard let item = item(selection), ["element", "selector", "match"].contains(item.node.kind) else { return nil }
        // a match node has the rules of its element
        let ruled = item.node.kind == "match" ? parentByID[item.id].flatMap { itemsByID[$0] } ?? item : item
        let rules = HeadMark.parse(ruled.node.rules)
        var failed: Set<String> = []
        for finding in (item.node.findings ?? []).compactMap({ $0 }) where finding.rule.hasPrefix("attr:") {
            if let at = finding.path.lastIndex(of: "@") {
                failed.insert(String(finding.path[finding.path.index(after: at)...]))
            }
        }
        var missing: String?
        if !item.node.found, item.node.kind == "element", item.node.path.hasPrefix("html/head/") {
            let tag = String(item.node.label.prefix { $0 != "[" })
            missing = "<" + tag + rules.literals.map { " \($0.key)=\"\($0.value)\"" }.joined() + ">"
        }
        return HeadMark(
            locators: item.node.found ? Set(item.node.locators ?? []) : [],
            color: item.node.status.color,
            attributes: Set(rules.literals.map(\.key)).union(rules.checked),
            failedAttributes: failed,
            missing: missing
        )
    }

    /// jsonMark marks the selected JSON-LD node in its document: a property
    /// path, or the objects of the type for a val:ld node
    var jsonMark: JSONMark? {
        guard let item = item(selection), ["ld", "match", "prop"].contains(item.node.kind) else { return nil }
        // up to the val:ld node - it knows the type, a match node of it knows the script
        var path: [String] = []
        var missing = false
        var current: TraceItem? = item
        var located: SemanticShapeVO.TraceNode?
        if item.node.kind == "prop" {
            // a missing property marks the object it is missing from
            missing = !item.node.found
            path = missing ? [] : item.node.label.split(separator: ".").map(String.init)
            current = parentByID[item.id].flatMap { itemsByID[$0] }
        }
        while let candidate = current, candidate.node.kind == "match" {
            located = located ?? candidate.node
            current = parentByID[candidate.id].flatMap { itemsByID[$0] }
        }
        guard let ld = current, ld.node.kind == "ld",
              let type = ld.node.label.firstRange(of: "(").map({ String(ld.node.label[$0.upperBound...].dropLast()) }),
              let locator = ((item.node.found ? item.node : nil) ?? located ?? ld.node).locators?.first,
              (located ?? ld.node).found else { return nil }
        return JSONMark(locator: locator, type: type, path: path, color: item.node.status.color, missing: missing)
    }

    /// select selects the most specific trace node that matched the page node
    /// at locator, and unfolds the tree down to it
    func select(locator: String) {
        var best: (id: TraceItem.ID, depth: Int)?
        func search(_ items: [TraceItem], depth: Int) {
            for item in items {
                // props share the locator of their script - the ld node is the one to show
                if item.node.found, item.node.kind != "prop", (item.node.locators ?? []).contains(locator),
                   depth > (best?.depth ?? -1) {
                    best = (item.id, depth)
                }
                search(item.children ?? [], depth: depth + 1)
            }
        }
        search(items, depth: 0)
        guard let best else { return }
        var parent = parentByID[best.id]
        while let id = parent {
            collapsed?.remove(id)
            parent = parentByID[id]
        }
        selection = best.id
    }

    /// pageDidFail is called by the web view if a page could not be loaded
    func pageDidFail(_ message: String) {
        error = message
    }

    /// loadOutline extracts the semantic outline of the page in the web view,
    /// without content: the server fetches it, or it gets the snapshot of the
    /// rendered DOM
    func loadOutline() async {
        guard highlighter.pageURL ?? pageURL != nil else {
            outline = .idle
            return
        }
        if source == .rendered, !highlighter.isPageLoaded {
            return
        }
        outline = .loading
        do {
            let document = try await fetchOutline(content: false)
            guard !Task.isCancelled else { return }
            outlineVersion += 1
            // a new outline - its elements may be gone
            if outlineSelection != nil {
                outlineSelection = nil
                highlightSelection()
            }
            outline = .loaded(document)
        } catch is CancellationError {
            return
        } catch {
            outline = .failed(error.localizedDescription)
        }
    }

    /// outlineContent extracts the outline with its content from the same
    /// source - for the branches the user opens
    func outlineContent() async throws -> WalkerVO.SemanticDocument {
        try await fetchOutline(content: true)
    }

    private struct OutlineError: LocalizedError {
        let errorDescription: String?
    }

    private func fetchOutline(content: Bool) async throws -> WalkerVO.SemanticDocument {
        guard let url = highlighter.pageURL ?? pageURL else {
            throw OutlineError(errorDescription: "no page")
        }
        var html = ""
        if source == .rendered {
            guard let snapshot = await highlighter.snapshot() else {
                throw OutlineError(errorDescription: "the page has no DOM yet")
            }
            html = snapshot
        }
        return try await client.getSemanticDocument(targetURL: url.absoluteString, html: html, content: content)
    }

    /// hoverOutline highlights an element of the outline in the page, nil
    /// brings back the highlight of the selection
    func hoverOutline(_ selector: String?) {
        guard let selector else {
            highlightSelection()
            return
        }
        Task {
            await highlighter.highlight(locators: [selector], color: Self.outlineColor, scroll: true)
        }
    }

    private static let outlineColor = "#af52de"

    /// highlightSelection highlights the selection in the page and gets the
    /// html of its elements: the element of the outline, else the shape node
    private func highlightSelection() {
        if let outline = outlineSelection {
            Task {
                await highlighter.highlight(locators: [outline.selector], color: Self.outlineColor, scroll: true)
                highlighted = nil
                let html = await highlighter.html(locators: [outline.selector])
                if outlineSelection == outline {
                    selectedHTML = html
                }
            }
            return
        }
        let node = item(selection)?.node
        Task {
            let result = await highlighter.highlight(node)
            highlighted = node == nil ? nil : result
            let html = node == nil ? nil : await highlighter.html(locators: node?.found == true ? node?.locators ?? [] : [])
            // a newer selection may have finished first
            if outlineSelection == nil, item(selection)?.node == node {
                selectedHTML = html
            }
        }
    }
}

/// TraceItem is a trace node in the outline - the id is the path of labels
/// from the root, stable across validations of the same shape
struct TraceItem: Identifiable, Hashable {
    let id: String
    let node: SemanticShapeVO.TraceNode
    let children: [TraceItem]?
    /// worst is the worst status of the node and its subtree
    let worst: SemanticShapeVO.TraceStatus
    /// chips and literals are computed once - rows render them on every scroll
    let chips: [RuleChip]
    let literals: [(key: String, value: String)]

    init(node: SemanticShapeVO.TraceNode, parentID: String, suffix: String = "") {
        let id = parentID + "/" + node.kind + ":" + node.label + suffix
        self.id = id
        self.node = node
        // labels can repeat among siblings, e.g. not allowed components in two places
        var seen: [String: Int] = [:]
        var children: [TraceItem] = []
        for child in (node.children ?? []).compactMap({ $0 }) {
            let key = child.kind + ":" + child.label
            let count = seen[key, default: 0]
            seen[key] = count + 1
            children.append(TraceItem(node: child, parentID: id, suffix: count == 0 ? "" : "#\(count)"))
        }
        self.children = children.isEmpty ? nil : children
        worst = children.reduce(node.status) { $1.worst.rank > $0.rank ? $1.worst : $0 }
        chips = RuleChip.chips(for: node)
        literals = RuleTokens.literals(node.rules)
    }

    /// equal includes the content - the ids are stable across validations, and
    /// SwiftUI must not take a row with new points or findings for unchanged
    static func == (lhs: TraceItem, rhs: TraceItem) -> Bool {
        lhs.id == rhs.id && lhs.node == rhs.node
    }

    func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }
}

extension SemanticShapeVO.TraceStatus {
    var rank: Int {
        switch self {
        case .error: 4
        case .warn: 3
        case .info: 2
        default: 0
        }
    }

    var isClean: Bool {
        rank < 3
    }
}

/// HeadMark marks a shape node among the head elements
struct HeadMark {
    /// locators of the head elements the node matched
    let locators: Set<String>
    let color: Color
    /// attributes the rules refer to - identifying literals and attr-* rules
    let attributes: Set<String>
    /// attributes with a failed attr rule
    let failedAttributes: Set<String>
    /// missing is the element as the shape describes it, if it is not in the page
    let missing: String?

    /// rule names of the shape - every other attribute of an element is a literal
    private static let ruleNames: Set<String> = [
        "min", "max", "count", "optional", "forbidden",
        "min-length", "max-length", "min-words", "max-words",
        "score", "score-per", "score-max", "severity", "penalty", "penalty-max", "open", "attr", "value",
    ]

    /// parse splits the rules of a trace node - key=value, key="quoted value"
    /// or key - into literal attributes and the attributes attr rules check
    static func parse(_ rules: String) -> (literals: [(key: String, value: String)], checked: Set<String>) {
        var literals: [(key: String, value: String)] = []
        var checked: Set<String> = []
        let pattern = /([^\s=]+)(?:=("(?:[^"\\]|\\.)*"|\S+))?/
        for match in rules.matches(of: pattern) {
            let key = String(match.output.1)
            var value = match.output.2.map(String.init) ?? ""
            if value.hasPrefix("\""), value.hasSuffix("\""), value.count >= 2 {
                value = String(value.dropFirst().dropLast()).replacingOccurrences(of: "\\\"", with: "\"")
            }
            if key.hasPrefix("attr-") {
                checked.insert(String(key.dropFirst("attr-".count)))
            } else if key == "attr" {
                // v1: attr="name; rules"
                checked.insert(value.split(separator: ";").first.map { $0.trimmingCharacters(in: .whitespaces) } ?? "")
            } else if !ruleNames.contains(key) {
                literals.append((key, value))
            }
        }
        return (literals, checked)
    }
}
