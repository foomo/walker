import AppKit
import SwiftUI

/// SchemaTreeView is the evaluated schema as an outline - clean subtrees are
/// folded, problems stay open. Elements from another schema file than their
/// parent get a heading with the file.
///
/// The selection is drawn neutral, like an unemphasized selection - the tree
/// drives the page and the middle column. A click on the selected row toggles
/// its details, the keyboard moves with ↑ ↓, folds with ← →, Return toggles.
struct SchemaTreeView: View {
    let items: [TraceItem]
    @Binding var selection: TraceItem.ID?
    @Binding var collapsed: Set<TraceItem.ID>
    /// showsDetails shows the details inline the selected row
    @AppStorage("schemaDevShowsDetails") private var showsDetails = true
    @FocusState private var focused: Bool

    var body: some View {
        ScrollViewReader { proxy in
            List {
                SchemaTreeRows(
                    items: items,
                    parentFile: items.first.map { TraceSource.file($0.node.source) } ?? "",
                    selection: selection,
                    showsDetails: showsDetails,
                    collapsed: $collapsed,
                    actions: TreeActions(
                        click: click,
                        closeDetails: { showsDetails = false }
                    )
                )
            }
            // the sidebar look, but on the neutral window background - the
            // sidebar material blends the desktop picture behind the window
            .listStyle(.sidebar)
            .scrollContentBackground(.hidden)
            .background(.windowBackground)
            .focusable()
            .focused($focused)
            .focusEffectDisabled()
            .onKeyPress(.downArrow) { move(1, proxy); return .handled }
            .onKeyPress(.upArrow) { move(-1, proxy); return .handled }
            .onKeyPress(.rightArrow) { fold(false); return .handled }
            .onKeyPress(.leftArrow) { fold(true); return .handled }
            .onKeyPress(.return) { showsDetails.toggle(); return .handled }
        }
    }

    /// click selects a row and opens its details, a click on the selected row toggles them
    private func click(_ id: TraceItem.ID) {
        focused = true
        if selection == id {
            showsDetails.toggle()
        } else {
            selection = id
            showsDetails = true
        }
    }

    /// visible are the rows in the order they are shown, folded subtrees left out
    private var visible: [TraceItem] {
        var result: [TraceItem] = []
        func walk(_ items: [TraceItem]) {
            for item in items {
                result.append(item)
                if let children = item.children, !collapsed.contains(item.id) {
                    walk(children)
                }
            }
        }
        walk(items)
        return result
    }

    private func move(_ step: Int, _ proxy: ScrollViewProxy) {
        let rows = visible
        guard !rows.isEmpty else { return }
        let current = rows.firstIndex { $0.id == selection } ?? (step > 0 ? -1 : rows.count)
        let next = min(max(current + step, 0), rows.count - 1)
        selection = rows[next].id
        proxy.scrollTo(rows[next].id)
    }

    private func fold(_ fold: Bool) {
        guard let selection, let item = visible.first(where: { $0.id == selection }), item.children != nil else { return }
        if fold {
            collapsed.insert(selection)
        } else {
            collapsed.remove(selection)
        }
    }
}

/// TreeActions are what the rows report back to the tree
private struct TreeActions {
    let click: (TraceItem.ID) -> Void
    let closeDetails: () -> Void
}

private struct SchemaTreeRows: View {
    let items: [TraceItem]
    /// parentFile is the schema file of the parent - a different file gets a heading
    let parentFile: String
    let selection: TraceItem.ID?
    let showsDetails: Bool
    @Binding var collapsed: Set<TraceItem.ID>
    let actions: TreeActions

    var body: some View {
        ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
            if let heading = heading(at: index) {
                OriginHeading(text: heading, file: TraceSource.file(item.node.source))
            }
            if let children = item.children {
                DisclosureGroup(isExpanded: Binding(
                    get: { !collapsed.contains(item.id) },
                    set: { expanded in
                        if expanded {
                            collapsed.remove(item.id)
                        } else {
                            collapsed.insert(item.id)
                        }
                    }
                )) {
                    SchemaTreeRows(items: children, parentFile: childFile(of: item, children: children), selection: selection, showsDetails: showsDetails, collapsed: $collapsed, actions: actions)
                } label: {
                    row(item, isCollapsed: collapsed.contains(item.id))
                }
                .id(item.id)
            } else {
                row(item, isCollapsed: false)
                    .id(item.id)
            }
        }
    }

    private func row(_ item: TraceItem, isCollapsed: Bool) -> TraceRow {
        let isSelected = item.id == selection
        return TraceRow(
            item: item,
            isCollapsed: isCollapsed,
            isSelected: isSelected,
            showsDetails: isSelected && showsDetails,
            actions: actions
        )
    }

    /// heading names the file of an item, if it differs from the one shown before it
    private func heading(at index: Int) -> String? {
        let file = TraceSource.file(items[index].node.source)
        let before = index == 0 ? parentFile : TraceSource.file(items[index - 1].node.source)
        guard !file.isEmpty, file != before else { return nil }
        return file == parentFile ? file : "from " + TraceSource.schemaPath(file)
    }

    /// childFile is the file the children of an item are read against - the
    /// body of a component is obviously from the component's file
    private func childFile(of item: TraceItem, children: [TraceItem]) -> String {
        if item.node.kind == "component", let first = children.first {
            return TraceSource.file(first.node.source)
        }
        return TraceSource.file(item.node.source)
    }
}

/// TraceSource reads the source of a trace node: file.html:line
enum TraceSource {
    static func file(_ source: String) -> String {
        guard let colon = source.lastIndex(of: ":"), source[source.index(after: colon)...].allSatisfy(\.isNumber) else {
            return source
        }
        return String(source[..<colon])
    }

    /// schemaPath is how schemas refer to a file: /partials/head
    static func schemaPath(_ file: String) -> String {
        "/" + (file.hasSuffix(".html") ? String(file.dropLast(5)) : file)
    }
}

/// OriginHeading separates elements of another schema file
private struct OriginHeading: View {
    let text: String
    let file: String
    @Environment(\.schemaRoot) private var root

    var body: some View {
        HStack(spacing: 6) {
            SourceLink(root: root, source: file, text: text)
                .font(.caption.monospaced())
            Rectangle()
                .fill(.separator)
                .frame(height: 1)
        }
        .padding(.top, 4)
        .selectionDisabled()
    }
}

/// TraceRow is one schema element in the tree: label, rule chips, score,
/// penalty, the findings of the element itself - and its details when selected
private struct TraceRow: View {
    let item: TraceItem
    let isCollapsed: Bool
    let isSelected: Bool
    let showsDetails: Bool
    let actions: TreeActions

    private var node: HTMLSchemaVO.TraceNode { item.node }

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                TraceLabel(item: item)
                    .lineLimit(1)
                if !node.via.isEmpty {
                    Text("via " + node.via)
                        .font(.caption2.monospaced())
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                }
                Spacer(minLength: 4)
                // a folded subtree shows its worst problem
                if isCollapsed, item.worst.rank > node.status.rank {
                    Circle()
                        .fill(item.worst.color)
                        .frame(width: 7, height: 7)
                        .help("Problems inside")
                }
                if node.max > 0 {
                    Text("\(node.points)/\(node.max)")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(node.points < node.max ? Color.secondary : Color.green)
                }
                if node.penalty > 0 {
                    Text("−\(node.penalty)")
                        .font(.caption.monospacedDigit().bold())
                        .foregroundStyle(.white)
                        .padding(.horizontal, 5)
                        .background(Capsule().fill(Color.red.opacity(0.85)))
                        .help("Penalty of this element and everything inside")
                }
            }
            if !item.chips.isEmpty {
                FlowLayout(spacing: 4) {
                    ForEach(item.chips) { chip in
                        RuleChipView(chip: chip)
                    }
                }
            }
            ForEach(Array((node.findings ?? []).compactMap { $0 }.enumerated()), id: \.offset) { _, finding in
                Text("\(finding.rule): \(finding.message)")
                    .font(.caption)
                    .foregroundStyle(HTMLSchemaVO.TraceStatus(rawValue: finding.severity.rawValue).color)
            }
            if showsDetails {
                SchemaNodeDetails(node: node, close: actions.closeDetails)
                    .padding(.top, 4)
            }
        }
        .padding(.vertical, 3)
        .padding(.horizontal, 5)
        .background {
            // a translucent neutral fill like a selected segment
            if isSelected {
                RoundedRectangle(cornerRadius: 8)
                    .fill(.quaternary)
            }
        }
        .contentShape(.rect)
        .onTapGesture { actions.click(item.id) }
    }
}

/// TraceLabel writes a node like a css-ish tag - `meta property=og:title` - or
/// with a kind tag for the special nodes of the schema language
struct TraceLabel: View {
    let item: TraceItem

    private var node: HTMLSchemaVO.TraceNode { item.node }

    var body: some View {
        switch node.kind {
        case "page":
            Text(TraceSource.file(node.source))
                .font(.body.monospaced())
        case "element", "match":
            Text(html)
                .font(.body.monospaced())
        case "ld":
            tagged("JSON-LD", .purple, Self.inner(node.label))
        case "selector":
            tagged("select", .blue, Self.inner(node.label))
        case "prop":
            tagged("prop", .secondary, node.label)
        case "component":
            tagged("component", .orange, node.label)
        case "choice", "sequence":
            tagged(node.kind, .teal, "")
        default:
            Text(node.label)
                .font(.body.monospaced())
        }
    }

    private func tagged(_ kind: String, _ color: Color, _ text: String) -> some View {
        HStack(spacing: 6) {
            Text(kind.uppercased())
                .font(.caption2.bold())
                .foregroundStyle(color)
                .padding(.horizontal, 5)
                .padding(.vertical, 1)
                .background(RoundedRectangle(cornerRadius: 4).fill(color.opacity(0.15)))
            if !text.isEmpty {
                Text(text)
                    .font(.body.monospaced())
            }
        }
    }

    /// inner is the text between the parentheses of val:ld(Product)
    private static func inner(_ label: String) -> String {
        guard let open = label.firstIndex(of: "("), label.hasSuffix(")") else { return label }
        return String(label[label.index(after: open)..<label.index(before: label.endIndex)])
    }

    /// html is the label as an html tag with its literal attributes - a match
    /// node gets its index
    private var html: AttributedString {
        let absent = node.status == .absent
        var label = node.label
        var index = ""
        if node.kind == "match", let open = label.lastIndex(of: "["), label.hasSuffix("]") {
            index = String(label[open...])
            label = String(label[..<open])
        }
        let tag = String(label.prefix { $0 != "[" })
        var result = AttributedString()
        func append(_ text: String, _ color: Color) {
            var part = AttributedString(text)
            part.foregroundColor = absent ? .secondary : color
            result += part
        }
        append(tag, .purple)
        for literal in item.literals {
            append(" " + literal.key, .secondary)
            if literal.value != "*" {
                append("=", .secondary)
                append(literal.value, .brown)
            }
        }
        if !index.isEmpty {
            append(" " + index, .secondary)
        }
        return result
    }
}

/// RuleTokens splits the rules of a trace node - key=value, key="quoted value" or key
enum RuleTokens {
    static let ruleNames: Set<String> = [
        "min", "max", "count", "optional", "forbidden",
        "min-length", "max-length", "min-words", "max-words",
        "score", "score-per", "score-max", "severity", "penalty", "penalty-max", "open", "attr", "value", "every",
    ]

    static func parse(_ rules: String) -> [(key: String, value: String)] {
        let pattern = /([^\s=]+)(?:=("(?:[^"\\]|\\.)*"|\S+))?/
        return rules.matches(of: pattern).map { match in
            var value = match.output.2.map(String.init) ?? ""
            if value.hasPrefix("\""), value.hasSuffix("\""), value.count >= 2 {
                value = String(value.dropFirst().dropLast()).replacingOccurrences(of: "\\\"", with: "\"")
            }
            return (String(match.output.1), value)
        }
    }

    /// literals are the attributes that identify an element - everything that is not a rule
    static func literals(_ rules: String) -> [(key: String, value: String)] {
        parse(rules).filter { !ruleNames.contains($0.key) && !$0.key.hasPrefix("attr-") }
    }
}

/// RuleChip is a rule of a node in plain language - failed if a finding of
/// the node is about it
struct RuleChip: Identifiable {
    enum Topic: Equatable {
        case occurrence, length, words, attribute(String), value, other
    }

    let id: String
    let text: String
    /// rule is the rule as written in the schema, for the tooltip
    let rule: String
    let topic: Topic
    var failed = false
    var quiet = false

    /// chips are computed once per validation - see TraceItem
    static func chips(for node: HTMLSchemaVO.TraceNode) -> [RuleChip] {
        let tokens = RuleTokens.parse(node.rules)
        var values: [String: String] = [:]
        for token in tokens {
            values[token.key] = token.value
        }
        func has(_ key: String) -> Bool { values[key] != nil }
        var chips: [RuleChip] = []

        // occurrence
        if has("forbidden") {
            chips.append(RuleChip(id: "occ", text: "forbidden", rule: "forbidden", topic: .occurrence))
        } else if let count = values["count"] {
            chips.append(RuleChip(id: "occ", text: "×" + count, rule: "count=" + count, topic: .occurrence))
        } else if has("min") || has("max") || has("optional") {
            let max = values["max"]
            let text: String
            switch (values["min"], max, has("optional")) {
            case (.some(let min), .some(let max), _) where max != "*":
                text = "×\(min)–\(max)"
            case (.some(let min), _, _):
                text = "×≥\(min)"
            case (.none, .some(let max), true) where max != "*":
                text = "optional ×≤\(max)"
            case (.none, _, true):
                text = "optional"
            case (.none, .some(let max), false):
                text = max == "*" ? "×≥1" : "×≤\(max)"
            default:
                text = ""
            }
            let rule = tokens.filter { ["min", "max", "optional"].contains($0.key) }.map { $0.value.isEmpty ? $0.key : "\($0.key)=\($0.value)" }.joined(separator: " ")
            chips.append(RuleChip(id: "occ", text: text, rule: rule, topic: .occurrence, quiet: has("optional")))
        }

        // text
        if let chip = range(values["min-length"], values["max-length"], unit: "chars") {
            chips.append(RuleChip(id: "length", text: chip, rule: "min-length / max-length", topic: .length))
        }
        if let chip = range(values["min-words"], values["max-words"], unit: "words") {
            chips.append(RuleChip(id: "words", text: chip, rule: "min-words / max-words", topic: .words))
        }

        // attributes and values
        for token in tokens {
            if token.key.hasPrefix("attr-") {
                let name = String(token.key.dropFirst(5))
                chips.append(RuleChip(id: token.key, text: name + " " + plain(token.value), rule: "\(token.key)=\(token.value)", topic: .attribute(name)))
            } else if token.key == "attr" {
                let parts = token.value.split(separator: ";", maxSplits: 1).map { $0.trimmingCharacters(in: .whitespaces) }
                let name = parts.first ?? ""
                chips.append(RuleChip(id: "attr", text: name + " " + plain(parts.count > 1 ? parts[1] : ""), rule: "attr=\(token.value)", topic: .attribute(name)))
            } else if token.key == "value" {
                chips.append(RuleChip(id: "value", text: plain(token.value), rule: "value=\(token.value)", topic: .value))
            }
        }

        // scoring and findings
        if let per = values["score-per"], let score = values["score"] {
            let parts = per.split(separator: ":")
            chips.append(RuleChip(id: "per", text: "\(score) pts / \(parts.last ?? "") \(parts.first ?? "")", rule: "score-per=\(per)", topic: .other, quiet: true))
        }
        if let max = values["score-max"] {
            chips.append(RuleChip(id: "smax", text: "≤\(max) pts", rule: "score-max=\(max)", topic: .other, quiet: true))
        }
        if let severity = values["severity"] {
            chips.append(RuleChip(id: "sev", text: severity, rule: "severity=\(severity)", topic: .other, quiet: true))
        }
        if let penalty = values["penalty"] {
            chips.append(RuleChip(id: "pen", text: "penalty \(penalty)", rule: "penalty=\(penalty)", topic: .other, quiet: true))
        }
        if has("every") {
            chips.append(RuleChip(id: "every", text: "every element", rule: "every", topic: .other))
        }
        if has("open") {
            chips.append(RuleChip(id: "open", text: "open", rule: "open", topic: .other, quiet: true))
        }

        // failed rules turn their chip red - a missing default occurrence gets a chip of its own
        for finding in (node.findings ?? []).compactMap({ $0 }) {
            let topic = topic(of: finding)
            if let index = chips.firstIndex(where: { $0.topic == topic }) {
                chips[index].failed = true
                chips[index].quiet = false
            } else if topic == .occurrence {
                let text = node.kind == "selector" ? "×≥1" : (finding.rule == "ld-prop:required" ? "required" : "×1")
                chips.insert(RuleChip(id: "occ", text: text, rule: "default occurrence", topic: .occurrence, failed: true), at: 0)
            }
        }
        return chips
    }

    private static func topic(of finding: HTMLSchemaVO.Finding) -> Topic {
        let rule = finding.rule.hasPrefix("ld-prop:") ? String(finding.rule.dropFirst(8)) : finding.rule
        switch rule {
        case "occurrence", "ld-occurrence", "required":
            return .occurrence
        case "min-length", "max-length":
            return .length
        case "min-words", "max-words":
            return .words
        default:
            if finding.rule.hasPrefix("attr:"), let at = finding.path.lastIndex(of: "@") {
                return .attribute(String(finding.path[finding.path.index(after: at)...]))
            }
            if finding.rule.hasPrefix("ld-prop:") {
                return .value
            }
            return .other
        }
    }

    private static func range(_ min: String?, _ max: String?, unit: String) -> String? {
        switch (min, max) {
        case let (min?, max?) where min == max:
            return "\(min) \(unit)"
        case let (min?, max?):
            return "\(min)–\(max) \(unit)"
        case let (min?, nil):
            return "≥\(min) \(unit)"
        case let (nil, max?):
            return "≤\(max) \(unit)"
        default:
            return nil
        }
    }

    /// plain writes a rule list like `min-length:70; max-length:160` in plain language
    static func plain(_ spec: String) -> String {
        var parts: [String] = []
        var rest = Substring(spec)
        var minLength: String?, maxLength: String?, minWords: String?, maxWords: String?
        while !rest.trimmingCharacters(in: .whitespaces).isEmpty {
            var part = rest
            rest = ""
            if !part.trimmingCharacters(in: .whitespaces).hasPrefix("regex:"), let semicolon = part.firstIndex(of: ";") {
                rest = part[part.index(after: semicolon)...]
                part = part[..<semicolon]
            }
            let rule = part.trimmingCharacters(in: .whitespaces)
            let name = rule.split(separator: ":", maxSplits: 1).first.map(String.init) ?? rule
            let arg = rule.contains(":") ? String(rule[rule.index(after: rule.firstIndex(of: ":")!)...]) : ""
            switch name {
            case "required": parts.append("required")
            case "empty": parts.append("empty")
            case "min-length": minLength = arg
            case "max-length": maxLength = arg
            case "length": parts.append("\(arg) chars")
            case "min-words": minWords = arg
            case "max-words": maxWords = arg
            case "one-of": parts.append(arg.replacingOccurrences(of: "|", with: " | "))
            case "url": parts.append(arg == "absolute" ? "absolute URL" : "URL")
            case "regex": parts.append("matches " + (arg.count > 28 ? String(arg.prefix(27)) + "…" : arg))
            case "severity": parts.append(arg)
            case "penalty": parts.append("penalty \(arg)")
            default: if !rule.isEmpty { parts.append(rule) }
            }
        }
        if let text = range(minLength, maxLength, unit: "chars") {
            parts.insert(text, at: 0)
        }
        if let text = range(minWords, maxWords, unit: "words") {
            parts.insert(text, at: 0)
        }
        return parts.joined(separator: " · ")
    }
}

private struct RuleChipView: View {
    let chip: RuleChip

    var body: some View {
        Text(chip.text)
            .font(.caption)
            .lineLimit(1)
            .padding(.horizontal, 7)
            .padding(.vertical, 1)
            .foregroundStyle(chip.failed ? Color.red : (chip.quiet ? Color.secondary : Color.primary))
            .background {
                if chip.failed {
                    Capsule().fill(Color.red.opacity(0.15))
                } else if chip.quiet {
                    Capsule().strokeBorder(.separator)
                } else {
                    Capsule().fill(.quaternary)
                }
            }
            .help(chip.rule)
    }
}

/// SchemaNodeDetails shows a node in full, inline its row: status, rules,
/// findings with their locators, the matched elements and the schema source
struct SchemaNodeDetails: View {
    let node: HTMLSchemaVO.TraceNode
    var close: (() -> Void)?
    @Environment(\.schemaRoot) private var root

    var body: some View {
        grid
            .padding(10)
            .background(RoundedRectangle(cornerRadius: 8).fill(Color(nsColor: .textBackgroundColor)))
            .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(.separator))
            .overlay(alignment: .topTrailing) {
                if let close {
                    Button(action: close) {
                        Image(systemName: "xmark.circle.fill")
                            .symbolRenderingMode(.hierarchical)
                            .foregroundStyle(.secondary)
                    }
                    .buttonStyle(.plain)
                    .padding(6)
                    .help("Close the details")
                }
            }
            // selectable text is expensive in scrolling lists - copy from the menu instead
            .contextMenu {
                if !node.path.isEmpty {
                    Button("Copy Path") { copy(node.path) }
                }
                if let locators = node.locators, !locators.isEmpty {
                    Button(locators.count == 1 ? "Copy Locator" : "Copy Locators") { copy(locators.joined(separator: "\n")) }
                }
                if !node.rules.isEmpty {
                    Button("Copy Rules") { copy(node.rules) }
                }
                if !node.source.isEmpty {
                    Button("Copy Source") { copy(node.source) }
                }
            }
    }

    private func copy(_ text: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
    }

    private var grid: some View {
        Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 5) {
            row("status") {
                Text(status(node))
                    .foregroundStyle(node.status.color)
            }
            if !node.rules.isEmpty {
                row("rules") { mono(node.rules) }
            }
            let findings = (node.findings ?? []).compactMap { $0 }
            ForEach(Array(findings.enumerated()), id: \.offset) { _, finding in
                row(finding.severity.rawValue) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("\(finding.rule): \(finding.message)")
                            .foregroundStyle(HTMLSchemaVO.TraceStatus(rawValue: finding.severity.rawValue).color)
                        mono(finding.locator)
                            .foregroundStyle(.secondary)
                        if !finding.source.isEmpty, finding.source != node.source {
                            SourceLink(root: root, source: finding.source)
                                .font(.caption.monospaced())
                        }
                    }
                }
            }
            let locators = node.locators ?? []
            if !locators.isEmpty {
                row(node.found ? "matched" : "missing in") {
                    VStack(alignment: .leading, spacing: 2) {
                        ForEach(Array(locators.prefix(8).enumerated()), id: \.offset) { _, locator in
                            mono(locator)
                        }
                        if locators.count > 8 {
                            Text("and \(locators.count - 8) more")
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            }
            if !node.via.isEmpty {
                row("via") { mono(node.via) }
            }
            if !node.source.isEmpty {
                row("source") {
                    SourceLink(root: root, source: node.source)
                        .font(.caption.monospaced())
                }
            }
            if !node.path.isEmpty {
                row("path") { mono(node.path) }
            }
        }
        .font(.caption)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func status(_ node: HTMLSchemaVO.TraceNode) -> String {
        var parts = [node.status.rawValue]
        if node.max > 0 {
            parts.append("\(node.points) / \(node.max) points")
        }
        if node.penalty > 0 {
            parts.append("penalty \(node.penalty)")
        }
        return parts.joined(separator: " · ")
    }

    private func row(_ title: String, @ViewBuilder _ content: () -> some View) -> some View {
        GridRow(alignment: .firstTextBaseline) {
            Text(title)
                .foregroundStyle(.secondary)
                .gridColumnAlignment(.trailing)
            content()
        }
    }

    private func mono(_ text: String) -> Text {
        Text(text).font(.caption.monospaced())
    }
}
