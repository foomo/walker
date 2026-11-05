import Foundation
import Observation
import SwiftUI

/// SchemaDevModel is the state of a schema developer window: a page, a schema
/// root and a group, validated on the walker server. Client and server share
/// a file system - the schema root is a path on the server.
@MainActor
@Observable
final class SchemaDevModel {
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
    var schemaRoot: String {
        didSet { save() }
    }
    /// group to validate with - "" is the group the page reports, then default.html
    var group: String {
        didSet { save() }
    }
    var source: Source {
        didSet { save() }
    }
    private(set) var recentRoots: [String]

    private(set) var rootInfo: WalkerVO.SchemaRootInfo?
    private(set) var validation: WalkerVO.SchemaValidation?
    /// items is the trace of the validation as a tree for the outline
    private(set) var items: [TraceItem] = []
    /// validatedLocators are the page nodes the schema matched
    private(set) var validatedLocators: Set<String> = []
    private(set) var isValidating = false
    private(set) var error: String?
    /// schemaReloadedAt is when a change of the schema files was picked up last
    private(set) var schemaReloadedAt: Date?
    /// highlighted is how many locators of the selection were found in the page - nil without selection
    private(set) var highlighted: (found: Int, total: Int)?
    /// pageVersion counts the pages the web view loaded - the outline reloads with it
    private(set) var pageVersion = 0
    /// outline is the semantic document of the page, from the same source as the validation
    private(set) var outline: OutlineState = .idle

    enum OutlineState {
        case idle
        case loading
        case loaded(WalkerVO.SemanticDocument)
        case failed(String)
    }

    /// selection is the id of the selected trace item
    var selection: TraceItem.ID? {
        didSet { highlightSelection() }
    }
    /// collapsed trace items - nil until the first trace collapsed the clean subtrees
    var collapsed: Set<TraceItem.ID>?

    @ObservationIgnored let highlighter = SchemaHighlighter()
    @ObservationIgnored private let client: Walker.ServiceClient
    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private var itemsByID: [TraceItem.ID: TraceItem] = [:]
    @ObservationIgnored private var parentByID: [TraceItem.ID: TraceItem.ID] = [:]
    /// validationCount identifies the latest validation - older ones that finish later are dropped
    @ObservationIgnored private var validationCount = 0

    private static let recentRootsKey = "schemaDevRecentRoots"

    init(server: Server, defaults: UserDefaults = .standard) {
        self.server = server
        self.defaults = defaults
        client = Walker.ServiceClient(transport: URLSessionTransport(baseURL: server.baseURL))
        let prefix = "schemaDev.\(server.id.uuidString)."
        address = defaults.string(forKey: prefix + "address") ?? ""
        schemaRoot = defaults.string(forKey: prefix + "schemaRoot") ?? ""
        group = defaults.string(forKey: prefix + "group") ?? ""
        source = Source(rawValue: defaults.string(forKey: prefix + "source") ?? "") ?? .server
        recentRoots = defaults.stringArray(forKey: Self.recentRootsKey) ?? []
    }

    private func save() {
        let prefix = "schemaDev.\(server.id.uuidString)."
        defaults.set(address, forKey: prefix + "address")
        defaults.set(schemaRoot, forKey: prefix + "schemaRoot")
        defaults.set(group, forKey: prefix + "group")
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

    /// reload reloads the page - pageDidLoad reloads the schema root and validates
    func reload() {
        guard pageURL != nil else {
            open()
            return
        }
        reloadCount += 1
    }

    /// chooseRoot sets the schema root and remembers it
    func chooseRoot(_ path: String) {
        schemaRoot = path
        recentRoots.removeAll { $0 == path }
        recentRoots.insert(path, at: 0)
        recentRoots = Array(recentRoots.prefix(8))
        defaults.set(recentRoots, forKey: Self.recentRootsKey)
    }

    /// load takes the site and the schema root of a walker config: the page
    /// is the base url with the first start path, a relative schema root is
    /// resolved against the directory of the config, like walker does
    func load(_ form: ConfigTool.Form, configURL: URL) {
        if !form.schemaRoot.isEmpty {
            let root = form.schemaRoot.hasPrefix("/")
                ? URL(filePath: form.schemaRoot)
                : configURL.deletingLastPathComponent().appending(path: form.schemaRoot)
            chooseRoot(root.standardizedFileURL.path(percentEncoded: false))
        }
        guard !form.baseURL.isEmpty else { return }
        var page = form.baseURL
        if let path = form.paths.first, !path.isEmpty, path != "/" {
            page = page.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + (path.hasPrefix("/") ? path : "/" + path)
        }
        address = page
        open()
    }

    func loadRoot() async {
        guard !schemaRoot.isEmpty else {
            rootInfo = nil
            return
        }
        do {
            let info = try await client.getSchemaRoot(schemaRoot: schemaRoot)
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
    /// watchSchema polls the fingerprint of the schema files on the server and
    /// validates again when one is edited, added or removed - it runs until
    /// the task is cancelled, e.g. by a new schema root
    func watchSchema() async {
        var version: String?
        while !Task.isCancelled {
            if !schemaRoot.isEmpty,
               let current = try? await client.getSchemaRootVersion(schemaRoot: schemaRoot),
               !current.isEmpty {
                if let version, version != current {
                    schemaReloadedAt = .now
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
        guard !schemaRoot.isEmpty else {
            error = "choose a schema root"
            return
        }
        // a page that is still loading has no DOM worth a snapshot - pageDidLoad validates it
        if source == .rendered, !highlighter.isPageLoaded {
            return
        }
        validationCount += 1
        let count = validationCount
        isValidating = true
        defer {
            if count == validationCount {
                isValidating = false
            }
        }
        do {
            var html = ""
            if source == .rendered {
                guard let snapshot = await highlighter.snapshot() else {
                    error = "the page has no DOM to validate yet"
                    return
                }
                html = snapshot
            }
            let validation = try await client.validateSchema(schemaRoot: schemaRoot, group: group, targetURL: url.absoluteString, html: html)
            guard !Task.isCancelled, count == validationCount else { return }
            apply(validation)
            error = nil
        } catch is CancellationError {
            return
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func apply(_ validation: WalkerVO.SchemaValidation) {
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

    /// headMark marks the selected schema node among the head elements: the
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
        var located: HTMLSchemaVO.TraceNode?
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

    /// loadOutline extracts the semantic document of the page in the web view:
    /// the server fetches it, or it gets the snapshot of the rendered DOM
    func loadOutline(content: Bool) async {
        guard let url = highlighter.pageURL ?? pageURL else {
            outline = .idle
            return
        }
        if source == .rendered, !highlighter.isPageLoaded {
            return
        }
        outline = .loading
        do {
            var html = ""
            if source == .rendered {
                guard let snapshot = await highlighter.snapshot() else {
                    outline = .failed("the page has no DOM yet")
                    return
                }
                html = snapshot
            }
            let document = try await client.getSemanticDocument(targetURL: url.absoluteString, html: html, content: content)
            guard !Task.isCancelled else { return }
            outline = .loaded(document)
        } catch is CancellationError {
            return
        } catch {
            outline = .failed(error.localizedDescription)
        }
    }

    /// hoverOutline highlights an element of the outline in the page, nil
    /// brings back the highlight of the selected schema node
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

    private func highlightSelection() {
        let node = item(selection)?.node
        Task {
            let result = await highlighter.highlight(node)
            highlighted = node == nil ? nil : result
        }
    }
}

/// TraceItem is a trace node in the outline - the id is the path of labels
/// from the root, stable across validations of the same schema
struct TraceItem: Identifiable, Hashable {
    let id: String
    let node: HTMLSchemaVO.TraceNode
    let children: [TraceItem]?
    /// worst is the worst status of the node and its subtree
    let worst: HTMLSchemaVO.TraceStatus
    /// chips and literals are computed once - rows render them on every scroll
    let chips: [RuleChip]
    let literals: [(key: String, value: String)]

    init(node: HTMLSchemaVO.TraceNode, parentID: String, suffix: String = "") {
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

extension HTMLSchemaVO.TraceStatus {
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

/// HeadMark marks a schema node among the head elements
struct HeadMark {
    /// locators of the head elements the node matched
    let locators: Set<String>
    let color: Color
    /// attributes the rules refer to - identifying literals and attr-* rules
    let attributes: Set<String>
    /// attributes with a failed attr rule
    let failedAttributes: Set<String>
    /// missing is the element as the schema describes it, if it is not in the page
    let missing: String?

    /// rule names of the schema - every other attribute of an element is a literal
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
