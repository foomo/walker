import AppKit
import SwiftUI

/// PageDataPane shows the invisible part of the validated html: the html
/// element, the head and the JSON-LD documents. Rows are linked with the
/// shape tree by their locators - a selected shape node marks the rows it
/// matched in its status color, clicking a row selects the shape node that
/// matched it.
struct PageDataPane: View {
    let model: ShapeDevModel
    @State private var collapsedBlocks: Set<String> = []
    /// showsAll shows every head element and JSON-LD document, not only the validated ones
    @AppStorage("shapeDevPageDataShowsAll") private var showsAll = false

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            if let page = model.validation?.page {
                let headMark = model.headMark
                let openGraph = OpenGraph(page: page)
                // the head elements of the link preview are shown with it, not in the head
                let previewLocators = openGraph?.previewLocators ?? []
                let missingInPreview = headMark?.missing.map(OpenGraph.isPreviewSource) ?? false
                ScrollViewReader { proxy in
                    List {
                        if let html = page.html {
                            Section("Document") {
                                headRow(html, mark: headMark)
                            }
                        }
                        if let openGraph {
                            Section("Link Preview") {
                                OpenGraphCard(openGraph: openGraph, model: model)
                                if missingInPreview, let missing = headMark?.missing {
                                    MissingHeadRow(source: missing, color: headMark?.color ?? .secondary)
                                        .id(Self.missingID)
                                }
                                ForEach((page.head ?? []).compactMap { $0 }.filter { previewLocators.contains($0.locator) }, id: \.locator) { element in
                                    headRow(element, mark: headMark)
                                }
                            }
                        }
                        Section("Head") {
                            if !missingInPreview || openGraph == nil, let missing = headMark?.missing {
                                MissingHeadRow(source: missing, color: headMark?.color ?? .secondary)
                                    .id(Self.missingID)
                            }
                            ForEach(headElements(page).filter { !previewLocators.contains($0.locator) }, id: \.locator) { element in
                                headRow(element, mark: headMark)
                            }
                        }
                        Section("JSON-LD") {
                            let all = (page.linkedData ?? []).compactMap { $0 }
                            let blocks = all.filter { isShown($0.locator) }
                            if blocks.isEmpty {
                                Text(all.isEmpty ? "No application/ld+json scripts" : "\(all.count) not validated by the shape")
                                    .foregroundStyle(.secondary)
                            }
                            ForEach(blocks, id: \.locator) { block in
                                LDBlockRow(block: block, mark: model.jsonMark.flatMap { $0.locator == block.locator ? $0 : nil }, isExpanded: Binding(
                                    get: { !collapsedBlocks.contains(block.locator) },
                                    set: { expanded in
                                        if expanded {
                                            collapsedBlocks.remove(block.locator)
                                        } else {
                                            collapsedBlocks.insert(block.locator)
                                        }
                                    }
                                ), onSelect: { model.select(locator: block.locator) })
                                .listRowBackground(model.jsonMark?.locator == block.locator ? model.jsonMark?.color.opacity(0.12) : nil)
                                .id(block.locator)
                            }
                        }
                    }
                    .onChange(of: model.selection) {
                        // a marked JSON-LD document has to be open to show the mark
                        if let mark = model.jsonMark {
                            collapsedBlocks.remove(mark.locator)
                        }
                        if model.headMark?.missing != nil {
                            withAnimation {
                                proxy.scrollTo(Self.missingID, anchor: .center)
                            }
                        } else if let first = model.selectedLocators.first ?? model.jsonMark?.locator {
                            withAnimation {
                                proxy.scrollTo(first, anchor: .center)
                            }
                        }
                    }
                }
            } else {
                ContentUnavailableView("No Page Data", systemImage: "doc.text.magnifyingglass", description: Text("The head and the JSON-LD of the validated html show up here."))
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .frame(maxHeight: .infinity, alignment: .top)
    }

    private var header: some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                Text("Head & JSON-LD")
                    .font(.headline)
                if let validation = model.validation {
                    Text(validation.source == "server" ? "fetched by the server" : "rendered DOM")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Spacer()
            Picker("Show", selection: $showsAll) {
                Text("Validated").tag(false)
                Text("All").tag(true)
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .fixedSize()
            .help("Validated: title, description, canonical and what the shape validates - All: the complete head and every JSON-LD document")
        }
        .padding(10)
    }

    private func isShown(_ locator: String) -> Bool {
        showsAll || model.validatedLocators.contains(locator)
    }

    /// headElements are the basics - title, description, canonical - and what the shape validated
    private func headElements(_ page: SemanticShapeVO.PageData) -> [SemanticShapeVO.HeadElement] {
        (page.head ?? []).compactMap { $0 }.filter { element in
            isShown(element.locator) || Self.isBasic(element)
        }
    }

    private static func isBasic(_ element: SemanticShapeVO.HeadElement) -> Bool {
        let attributes = element.attributes ?? []
        func value(_ key: String) -> String {
            attributes.first { $0.key == key }?.value.lowercased() ?? ""
        }
        switch element.tag {
        case "title":
            return true
        case "meta":
            return value("name") == "description"
        case "link":
            return value("rel").split(separator: " ").contains("canonical")
        default:
            return false
        }
    }

    private static let missingID = "#missing"

    private func headRow(_ element: SemanticShapeVO.HeadElement, mark: HeadMark?) -> some View {
        let isMarked = mark?.locators.contains(element.locator) ?? false
        return HeadRow(
            element: element,
            isValidated: element.tag == "html" || model.validatedLocators.contains(element.locator),
            mark: isMarked ? mark : nil
        )
        .contentShape(.rect)
        .onTapGesture { model.select(locator: element.locator) }
        .listRowBackground(isMarked ? mark?.color.opacity(0.15) : nil)
        .id(element.locator)
    }
}

/// MissingHeadRow is a head element the selected shape node did not find -
/// dashed like the outline of a missing element in the page
private struct MissingHeadRow: View {
    let source: String
    let color: Color

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(source)
                .font(.callout.monospaced())
            Text("missing in the page")
                .font(.caption)
                .foregroundStyle(color)
        }
        .padding(6)
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay {
            RoundedRectangle(cornerRadius: 4)
                .strokeBorder(color, style: StrokeStyle(lineWidth: 1.5, dash: [4, 3]))
        }
    }
}

/// HeadRow is an element of the head, written like in the html source
private struct HeadRow: View {
    let element: SemanticShapeVO.HeadElement
    /// isValidated is false for a basic element the shape does not check
    let isValidated: Bool
    /// mark of the selected shape node, if it matched this element
    let mark: HeadMark?

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            source
                .font(.callout.monospaced())
                .lineLimit(4)
            if !element.text.isEmpty {
                Text(element.text)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(3)
            }
            if !isValidated {
                if isExcludedFromMatching {
                    Label("excluded from semantic matching", systemImage: "minus.circle")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .help("Walker excludes scripts and styles from its semantic tree. JSON-LD is validated separately.")
                } else {
                    Label("not in the shape", systemImage: "questionmark.circle")
                        .font(.caption)
                        .foregroundStyle(.orange)
                }
            }
        }
        .help(element.locator)
        .contextMenu {
            Button("Copy") {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(html, forType: .string)
            }
            Button("Copy Locator") {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(element.locator, forType: .string)
            }
        }
    }

    private var attributes: [SemanticShapeVO.HeadAttribute] {
        element.attributes ?? []
    }

    private var isExcludedFromMatching: Bool {
        if ["style", "template", "noscript"].contains(element.tag) { return true }
        let type = attributes.first { $0.key == "type" }?.value.lowercased() ?? ""
        return element.tag == "script" && type != "application/ld+json"
    }

    /// source shows the tag purple, attribute names secondary and the values -
    /// the attributes the selected shape node checks get a background
    private var source: Text {
        var result = AttributedString()
        func append(_ text: String, _ color: Color? = nil, background: Color? = nil) {
            var part = AttributedString(text)
            if let color {
                part.foregroundColor = color
            }
            if let background {
                part.backgroundColor = background
            }
            result += part
        }
        append("<" + element.tag, .purple)
        for attribute in attributes {
            append(" ")
            var background: Color?
            if let mark, mark.failedAttributes.contains(attribute.key) {
                background = Color.red.opacity(0.35)
            } else if let mark, mark.attributes.contains(attribute.key) {
                background = mark.color.opacity(0.3)
            }
            append(attribute.key + "=", .secondary, background: background)
            append("\"" + attribute.value + "\"", background: background)
        }
        append(">", .purple)
        return Text(result)
    }

    private var html: String {
        "<" + element.tag + attributes.map { " \($0.key)=\"\($0.value)\"" }.joined() + ">"
    }
}

/// LDBlockRow is a JSON-LD document, indented, under its types
private struct LDBlockRow: View {
    let block: SemanticShapeVO.LDBlock
    /// mark is the selected shape node in this document
    let mark: JSONMark?
    @Binding var isExpanded: Bool
    let onSelect: () -> Void

    var body: some View {
        DisclosureGroup(isExpanded: $isExpanded) {
            Text(JSONHighlighter.highlight(block.json, mark: mark))
                .font(.caption.monospaced())
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
        } label: {
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.callout.monospaced().bold())
                    .contentShape(.rect)
                    .onTapGesture(perform: onSelect)
                if !block.error.isEmpty {
                    Label(block.error, systemImage: "xmark.octagon")
                        .font(.caption)
                        .foregroundStyle(.red)
                }
            }
        }
        .help(block.locator)
        .contextMenu {
            Button("Copy JSON") {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(block.json, forType: .string)
            }
        }
    }

    private var title: String {
        let types = block.types ?? []
        return types.isEmpty ? "JSON-LD" : types.joined(separator: " · ")
    }
}
