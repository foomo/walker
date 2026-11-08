import AppKit
import SwiftUI

/// ShapeDevView iterates on a shape and a website of a server - the Shapes
/// section of the server: the evaluated shape as a tree next to the page, a
/// selected node is highlighted in the page. A local server brings the shape
/// root and the site of its config.
struct ShapeDevView: View {
    @Environment(ServerStore.self) private var store
    @Bindable var model: ShapeDevModel
    /// showsPageData shows the head and the JSON-LD between the shape and the page
    @AppStorage("shapeDevShowsPageData") private var showsPageData = true
    /// showsOutline shows the semantic outline of the page right of it
    @AppStorage("shapeDevShowsOutline") private var showsOutline = false
    /// showsElementHTML shows the html of the selected elements under the page
    @AppStorage("shapeDevShowsElementHTML") private var showsElementHTML = true

    /// followed is set once a local server brought the shape root of its
    /// config - until then the prompt for a root would flash
    @State private var followed = false

    var body: some View {
        Group {
            if !followed {
                ProgressView()
                    .controlSize(.small)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if model.shapeRoot.isEmpty {
                ShapeRootPrompt(model: model)
            } else {
                developer
            }
        }
        .task(id: model.shapeRoot) {
            await model.loadRoot()
            // edits to the shape files validate again, until the root changes
            await model.watchShape()
        }
        .onChange(of: model.group) {
            Task { await model.validate() }
        }
        .onChange(of: model.source) {
            Task { await model.validate() }
        }
        .onChange(of: model.shapeRoot) {
            Task { await model.validate() }
        }
        .onChange(of: model.groupHeader) {
            Task { await model.validate() }
        }
        // a local server brings the shape root of its config - and its site, until another page is open
        .task {
            if let url = model.server.configURL, let result = try? await store.refreshLocal(model.server.id) {
                model.follow(result.form, configURL: url)
            }
            followed = true
            model.open()
        }
    }

    /// developer is the bar and the panes - shown once there is a shape root
    private var developer: some View {
        VStack(spacing: 0) {
            bar
            Divider()
            // every column takes the full height - small content like a lint
            // error would otherwise be centered vertically by the split view
            HSplitView {
                ShapePane(model: model)
                    .frame(minWidth: 320, idealWidth: 440, maxWidth: 700, maxHeight: .infinity, alignment: .top)
                if showsPageData {
                    PageDataPane(model: model)
                        .frame(minWidth: 280, idealWidth: 380, maxWidth: 700, maxHeight: .infinity, alignment: .top)
                }
                page
                    .frame(minWidth: 420, maxWidth: .infinity, maxHeight: .infinity)
                if showsOutline {
                    OutlinePane(model: model)
                        .frame(minWidth: 280, idealWidth: 380, maxWidth: 700, maxHeight: .infinity, alignment: .top)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    /// page is the web view - with the html of the selected elements under it
    private var page: some View {
        VSplitView {
            webPage
                .frame(minHeight: 200, maxHeight: .infinity)
            if showsElementHTML {
                ElementHTMLPane(model: model)
                    .frame(minHeight: 100, idealHeight: 220, maxHeight: .infinity)
            }
        }
    }

    @ViewBuilder
    private var webPage: some View {
        if model.pageURL == nil {
            ContentUnavailableView("No Page", systemImage: "globe", description: Text("Enter the url of a page to validate."))
        } else {
            ShapeWebView(
                url: model.pageURL,
                reloadCount: model.reloadCount,
                highlighter: model.highlighter,
                onLoad: { url in Task { await model.pageDidLoad(url) } },
                onFail: { message in model.pageDidFail(message) }
            )
        }
    }

    /// bar holds the controls of the section: the page, the shape root and
    /// group it is validated with, the editor, the panes and the source
    private var bar: some View {
        HStack(spacing: 10) {
            Button("Reload", systemImage: "arrow.clockwise") {
                model.reload()
            }
            .labelStyle(.iconOnly)
            .keyboardShortcut("r")
            .help("Reload the page and the shape, then validate again (⌘R)")
            AddressField(text: $model.address) { model.open() }
                .frame(minWidth: 220, maxWidth: .infinity)
            ShapeRootMenu(model: model)
                .fixedSize()
            Picker("Group", selection: $model.group) {
                Text(autoGroupTitle).tag("")
                if let groups = model.rootInfo?.groups, !groups.isEmpty {
                    Divider()
                    ForEach(groups, id: \.self) { group in
                        Text(group).tag(group)
                    }
                }
            }
            .labelsHidden()
            .fixedSize()
            .help("The group shape to validate with")
            GroupHeaderButton(model: model)
                .fixedSize()
            Picker("Source", selection: $model.source) {
                ForEach(ShapeDevModel.Source.allCases) { source in
                    Text(source.rawValue).tag(source)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .fixedSize()
            .help("Server: the server fetches the page like the crawler - Rendered: a snapshot of the DOM in the web view")
            Divider()
                .frame(height: 18)
            EditorMenu()
                .fixedSize()
            Toggle(isOn: $showsPageData) {
                Label("Head & JSON-LD", systemImage: "curlybraces.square")
            }
            .help(showsPageData ? "Hide the head and the JSON-LD of the validated html" : "Show the head and the JSON-LD of the validated html")
            Toggle(isOn: $showsElementHTML) {
                Label("Element HTML", systemImage: "chevron.left.forwardslash.chevron.right")
            }
            .help(showsElementHTML ? "Hide the html of the selected elements" : "Show the html of the elements the selected shape node matched, under the page")
            Toggle(isOn: $showsOutline) {
                Label("Document Outline", systemImage: "list.bullet.indent")
            }
            .help(showsOutline ? "Hide the outline of the page" : "Show the outline of the page: landmarks, articles, sections, headings - and their content")
        }
        .toggleStyle(.button)
        .labelStyle(.iconOnly)
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
    }

    /// autoGroupTitle names the group auto resolves to
    private var autoGroupTitle: String {
        if let detected = model.validation?.detectedGroup, !detected.isEmpty {
            return "Auto (\(detected))"
        }
        if !model.effectiveGroupHeader.isEmpty {
            let header = model.effectiveGroupHeader
            return "Auto (header \(header))"
        }
        return "Auto"
    }
}

/// GroupHeaderButton shows the response header the group of a page is read
/// from - a click edits it. A local server keeps it in its config as
/// groupheader, for a remote one it overrides the server's in this app.
private struct GroupHeaderButton: View {
    @Bindable var model: ShapeDevModel
    @Environment(ServerStore.self) private var store
    @State private var isEditing = false
    @State private var draft = ""
    @State private var error: String?
    @State private var isSaving = false

    private var isLocal: Bool { model.server.configURL != nil }
    /// serverHeader is the header of the running walker's config
    private var serverHeader: String { model.rootInfo?.groupHeader ?? "" }

    var body: some View {
        Button {
            draft = model.effectiveGroupHeader
            error = nil
            isEditing = true
        } label: {
            Label(model.effectiveGroupHeader.isEmpty ? "No Group Header" : model.effectiveGroupHeader, systemImage: "tag")
                .labelStyle(.titleAndIcon)
                .foregroundStyle(model.effectiveGroupHeader.isEmpty ? .secondary : .primary)
        }
        .help(model.effectiveGroupHeader.isEmpty
            ? "No response header names the group of a page - every page is validated with default.semanticshape.html unless a group is chosen. Click to set one."
            : "The response header \(model.effectiveGroupHeader) names the group of a page - click to edit")
        .popover(isPresented: $isEditing, arrowEdge: .bottom) {
            editor
        }
    }

    private var editor: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Group Header")
                .font(.headline)
            TextField("Group header", text: $draft, prompt: Text("x-next-js-app"))
                .textFieldStyle(.roundedBorder)
                .frame(width: 260)
                .onSubmit(save)
            Text(note)
                .font(.caption)
                .foregroundStyle(.secondary)
                .frame(width: 260, alignment: .leading)
                .fixedSize(horizontal: false, vertical: true)
            if let error {
                Label(error, systemImage: "exclamationmark.triangle")
                    .font(.caption)
                    .foregroundStyle(.red)
                    .frame(width: 260, alignment: .leading)
                    .fixedSize(horizontal: false, vertical: true)
            }
            HStack {
                if !isLocal, !model.groupHeader.isEmpty {
                    Button("Use the Server's") {
                        model.groupHeader = ""
                        isEditing = false
                    }
                    .help(serverHeader.isEmpty ? "The server's config has no group header" : "The server's config has \(serverHeader)")
                }
                Spacer()
                Button("Cancel") { isEditing = false }
                    .keyboardShortcut(.cancelAction)
                Button(isLocal ? "Save to Config" : "Use") { save() }
                    .keyboardShortcut(.defaultAction)
                    .disabled(isSaving)
            }
        }
        .padding(14)
    }

    private var note: String {
        if isLocal {
            return "The response header that names the group of a page, e.g. x-next-js-app: product validates with groups/product.semanticshape.html. Saved to the config as groupheader - the validation here uses it right away, a running walker after a restart."
        }
        return "The response header that names the group of a page, e.g. x-next-js-app: product validates with groups/product.semanticshape.html. Used for the validation here, the server's config stays as it is" + (serverHeader.isEmpty ? " (it has none)." : " (\(serverHeader)).")
    }

    private func save() {
        let header = draft.trimmingCharacters(in: .whitespaces)
        guard isLocal else {
            model.groupHeader = header == serverHeader ? "" : header
            isEditing = false
            return
        }
        isSaving = true
        error = nil
        Task {
            defer { isSaving = false }
            do {
                try await store.setGroupHeader(model.server.id, header: header)
                model.groupHeader = header
                isEditing = false
            } catch {
                self.error = "The config could not be written: \(error.localizedDescription)"
            }
        }
    }
}

/// ShapeRootPanel lets the user choose a shape root folder
enum ShapeRootPanel {
    /// choose returns the chosen folder - nil if the user cancelled. from is
    /// where the panel starts, a folder or "".
    @MainActor
    static func choose(from: String) -> String? {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.canCreateDirectories = true
        panel.allowsMultipleSelection = false
        panel.prompt = "Choose"
        panel.message = "Choose a shape root - the folder with default.semanticshape.html and groups/"
        if !from.isEmpty {
            panel.directoryURL = URL(filePath: from)
        }
        guard panel.runModal() == .OK, let url = panel.url else { return nil }
        return url.path(percentEncoded: false)
    }
}

/// ShapeRootPrompt is all the section shows without a shape root: the shapes
/// a page is validated with are the first thing it needs. For a local server
/// the folder goes into its config as shaperoot - the walker validates its
/// scrapes with it too.
private struct ShapeRootPrompt: View {
    let model: ShapeDevModel
    @Environment(ServerStore.self) private var store
    @State private var error: String?
    @State private var isWriting = false

    private var isLocal: Bool { model.server.configURL != nil }

    var body: some View {
        ContentUnavailableView {
            Label("No Shape Root", systemImage: "folder.badge.questionmark")
        } description: {
            if isLocal {
                Text("Choose the folder with the semantic shapes of \(model.server.name). It is written to the config as `shaperoot` - a running walker validates its scrapes with it after a restart.")
            } else {
                Text("Choose the folder with the semantic shapes of \(model.server.name) - a path the server can read, it shares the file system with this app.")
            }
        } actions: {
            Button {
                if let path = ShapeRootPanel.choose(from: model.recentRoots.first ?? model.server.configURL?.deletingLastPathComponent().path(percentEncoded: false) ?? "") {
                    use(path)
                }
            } label: {
                Label("Choose Shape Root…", systemImage: "folder")
            }
            .buttonStyle(.borderedProminent)
            .disabled(isWriting)
            if !model.recentRoots.isEmpty {
                Menu("Recent") {
                    ForEach(model.recentRoots, id: \.self) { root in
                        Button(root) { use(root) }
                    }
                }
                .fixedSize()
                .disabled(isWriting)
            }
            if let error {
                Label(error, systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.red)
            }
        }
    }

    /// use takes the folder - for a local server it is written to the config first
    private func use(_ path: String) {
        guard isLocal else {
            model.chooseRoot(path)
            return
        }
        isWriting = true
        error = nil
        Task {
            defer { isWriting = false }
            do {
                try await store.setShapeRoot(model.server.id, path: path)
                if let url = model.server.configURL {
                    let result = try await store.refreshLocal(model.server.id)
                    model.follow(result.form, configURL: url)
                }
                if model.shapeRoot.isEmpty {
                    model.chooseRoot(path)
                }
            } catch {
                self.error = "The config could not be written: \(error.localizedDescription)"
            }
        }
    }
}

/// ShapeRootMenu picks the shape root - a directory on the server, which
/// shares the file system with this app
private struct ShapeRootMenu: View {
    @Bindable var model: ShapeDevModel

    var body: some View {
        Menu {
            if let configRoot = model.configRoot {
                Section("From the Config") {
                    Button(configRoot) { model.chooseRoot(configRoot) }
                }
            }
            let recent = model.recentRoots.filter { $0 != model.configRoot }
            if !recent.isEmpty {
                Section("Recent") {
                    ForEach(recent, id: \.self) { root in
                        Button(root) { model.chooseRoot(root) }
                    }
                }
            }
            Divider()
            Button("Choose…") { choose() }
        } label: {
            Label(model.shapeRoot.isEmpty ? "Shape Root" : (model.shapeRoot as NSString).lastPathComponent, systemImage: "folder")
                .labelStyle(.titleAndIcon)
        }
        .help(help)
    }

    private var help: String {
        if model.shapeRoot.isEmpty {
            return "Choose the shape root"
        }
        if model.shapeRoot == model.configRoot {
            return "\(model.shapeRoot) - the shape root of the config of this server"
        }
        return model.shapeRoot
    }

    private func choose() {
        if let path = ShapeRootPanel.choose(from: model.shapeRoot) {
            model.chooseRoot(path)
        }
    }
}

/// OutlinePane shows the semantic outline of the page, from the same source as
/// the validation - hovering an element highlights it in the page
private struct OutlinePane: View {
    let model: ShapeDevModel

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            content
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        }
        // a new page or source loads again - shape edits don't change the page
        .task(id: "\(model.pageVersion) \(model.source.rawValue)") {
            await model.loadOutline()
        }
    }

    private var header: some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                Text("Document Outline")
                    .font(.headline)
                Text(model.source == .server ? "fetched by the server" : "rendered DOM")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
        }
        .padding(10)
        .help("Click an element to show the content of its branch: texts, links, images, lists and tables")
    }

    @ViewBuilder
    private var content: some View {
        switch model.outline {
        case .idle:
            ContentUnavailableView("No Page", systemImage: "globe", description: Text("Load a page to see its outline."))
        case .loading:
            ProgressView()
                .controlSize(.small)
                .padding()
        case let .failed(message):
            Label(message, systemImage: "exclamationmark.triangle")
                .foregroundStyle(.red)
                .padding(10)
        case let .loaded(result):
            ScrollView {
                VStack(alignment: .leading, spacing: 8) {
                    SemanticDocumentContent(result: result, pageURL: model.highlighter.pageURL ?? model.pageURL, loadContent: { [model] in
                        try await model.outlineContent()
                    }, marked: model.selectedLocators, selected: model.outlineSelection?.selector, onSelect: { [model] selector, element in
                        model.selectOutline(selector, label: element.outlineLabel)
                    }) { selector, _ in
                        model.hoverOutline(selector)
                    }
                    .id(model.outlineVersion)
                }
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .textSelection(.enabled)
            }
        }
    }
}

/// ElementHTMLPane shows the html of the elements the selected shape node
/// matched - from the page in the web view, the one that is highlighted
private struct ElementHTMLPane: View {
    let model: ShapeDevModel

    private var node: SemanticShapeVO.TraceNode? { model.item(model.selection)?.node }

    /// title names the selection: the element of the outline, else the shape node
    private var title: String? { model.outlineSelection?.label ?? node?.label }

    var body: some View {
        VStack(spacing: 0) {
            HStack(alignment: .firstTextBaseline) {
                Text("Element HTML")
                    .font(.headline)
                if let title {
                    Text(title)
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                }
                Spacer()
                Button("Copy", systemImage: "doc.on.doc") {
                    copy()
                }
                .labelStyle(.iconOnly)
                .buttonStyle(.borderless)
                .disabled(html.isEmpty)
                .help("Copy the html")
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
            Divider()
            content
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        }
    }

    /// html is the found elements as one text
    private var html: String {
        (model.selectedHTML ?? []).compactMap(\.html).joined(separator: "\n\n")
    }

    @ViewBuilder
    private var content: some View {
        if model.outlineSelection != nil, let elements = model.selectedHTML {
            elementsView(elements)
        } else if model.outlineSelection != nil {
            ProgressView()
                .controlSize(.small)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let node, let elements = model.selectedHTML {
            if !node.found {
                note("The selected node matched no element - there is no html to show.")
            } else if elements.isEmpty {
                note("The selected node has no elements in the page.")
            } else {
                elementsView(elements)
            }
        } else if node != nil {
            ProgressView()
                .controlSize(.small)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            note("Select a node in the shape tree or an element of the outline to see its html.")
        }
    }

    /// elementsView shows the html of the elements, each with its locator if there are several
    private func elementsView(_ elements: [ShapeHighlighter.ElementHTML]) -> some View {
        ScrollView([.vertical, .horizontal]) {
            VStack(alignment: .leading, spacing: 10) {
                ForEach(elements, id: \.locator) { element in
                    VStack(alignment: .leading, spacing: 4) {
                        if elements.count > 1 {
                            Text(element.locator)
                                .font(.caption2.monospaced())
                                .foregroundStyle(.tertiary)
                        }
                        if let html = element.html {
                            Text(HTMLHighlighter.highlight(html))
                                .font(.caption.monospaced())
                                .fixedSize()
                            if element.truncated {
                                Text("… cut off - the element is too large to show in full")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        } else {
                            Text("Not in the page of the web view - with the source Server the crawler's html can differ from the DOM.")
                                .font(.caption)
                                .foregroundStyle(.orange)
                        }
                    }
                }
            }
            .padding(10)
            .textSelection(.enabled)
        }
    }

    private func note(_ text: String) -> some View {
        Text(text)
            .font(.callout)
            .foregroundStyle(.secondary)
            .padding(10)
    }

    private func copy() {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(html, forType: .string)
    }
}

/// ShapePane shows the result of the validation: summary, lint errors or the shape tree
private struct ShapePane: View {
    @Bindable var model: ShapeDevModel

    var body: some View {
        VStack(spacing: 0) {
            ShapeSummary(model: model)
            Divider()
            content
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    @ViewBuilder
    private var content: some View {
        let lintErrors = model.validation?.lintErrors ?? model.rootInfo?.lintErrors ?? []
        if !lintErrors.isEmpty {
            List(Array(lintErrors.enumerated()), id: \.offset) { _, lintError in
                VStack(alignment: .leading, spacing: 2) {
                    SourceLink(root: model.shapeRoot, source: "\(lintError.file):\(lintError.line)")
                        .font(.caption.monospaced())
                    Text(lintError.message)
                }
            }
        } else if model.shapeRoot.isEmpty {
            ContentUnavailableView("No Shape Root", systemImage: "folder", description: Text("Choose the shape root in the toolbar."))
        } else if model.items.isEmpty {
            ContentUnavailableView("No Validation", systemImage: "checklist", description: Text(model.isValidating ? "Validating…" : "Load a page to validate it."))
        } else {
            ShapeTreeView(items: model.items, selection: $model.selection, collapsed: Binding(
                get: { model.collapsed ?? [] },
                set: { model.collapsed = $0 }
            ))
            .environment(\.shapeRoot, model.shapeRoot)
        }
    }
}

/// ShapeSummary is the head of the shape pane: score, penalty and findings
private struct ShapeSummary: View {
    let model: ShapeDevModel

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            if let report = model.validation?.report {
                HStack(spacing: 12) {
                    if let score = report.score {
                        Label(score.summary, systemImage: "star")
                            .help("Score - points of the maximum")
                    }
                    Label("\(report.penalty)", systemImage: "exclamationmark.triangle")
                        .foregroundStyle(report.penalty > 0 ? Color.red : Color.secondary)
                        .help("Penalty - never subtracted from the score")
                    Spacer()
                    findingCount(report, .error)
                    findingCount(report, .warn)
                    findingCount(report, .info)
                }
                .monospacedDigit()
                Text(report.shape + sourceDescription + reloadDescription)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
            if let highlighted = model.highlighted, highlighted.found < highlighted.total {
                Label("\(highlighted.total - highlighted.found) of \(highlighted.total) not in the page - the page differs from the validated html", systemImage: "eye.slash")
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
            if let error = model.error ?? nonEmpty(model.validation?.error) ?? nonEmpty(model.rootInfo?.error) {
                Label(error, systemImage: "xmark.octagon")
                    .font(.caption)
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
            }
            if model.isValidating {
                ProgressView()
                    .controlSize(.small)
            }
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var reloadDescription: String {
        guard let reloaded = model.shapeReloadedAt else { return "" }
        return " · shape reloaded " + reloaded.formatted(date: .omitted, time: .standard)
    }

    private var sourceDescription: String {
        guard let validation = model.validation else { return "" }
        if validation.source == "server" {
            return " · fetched by the server, status \(validation.statusCode)"
        }
        return " · rendered DOM"
    }

    private func findingCount(_ report: SemanticShapeVO.Report, _ severity: SemanticShapeVO.Severity) -> some View {
        let count = (report.findings ?? []).filter { $0?.severity == severity }.count
        return Text("\(count) \(severity.rawValue)")
            .foregroundStyle(count > 0 ? SemanticShapeVO.TraceStatus(rawValue: severity.rawValue).color : Color.secondary)
    }

    private func nonEmpty(_ s: String?) -> String? {
        guard let s, !s.isEmpty else { return nil }
        return s
    }
}

/// AddressField is the url field of the toolbar, a capsule like Safari's - the
/// toolbar item clips everything outside its bounds, so the focus ring is
/// drawn on the inner edge of the capsule, not around it. @FocusState does not
/// report focus in toolbar items, so the field is an NSTextField that does
struct AddressField: View {
    @Binding var text: String
    let onSubmit: () -> Void
    @State private var isFocused = false

    var body: some View {
        AddressTextField(text: $text, isFocused: $isFocused, onSubmit: onSubmit)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .glassEffect(.regular, in: Capsule())
            .overlay {
                Capsule()
                    .strokeBorder(Color(nsColor: .keyboardFocusIndicatorColor), lineWidth: 3)
                    .opacity(isFocused ? 1 : 0)
            }
            .animation(.easeOut(duration: 0.15), value: isFocused)
    }
}

/// AddressTextField is a borderless NSTextField that reports whether it is
/// being edited
struct AddressTextField: NSViewRepresentable {
    @Binding var text: String
    @Binding var isFocused: Bool
    let onSubmit: () -> Void

    final class Field: NSTextField {
        var onFocus: (() -> Void)?

        override func becomeFirstResponder() -> Bool {
            let became = super.becomeFirstResponder()
            if became { onFocus?() }
            return became
        }
    }

    @MainActor
    final class Coordinator: NSObject, NSTextFieldDelegate {
        var parent: AddressTextField

        init(_ parent: AddressTextField) {
            self.parent = parent
        }

        func controlTextDidChange(_ notification: Notification) {
            guard let field = notification.object as? NSTextField else { return }
            parent.text = field.stringValue
        }

        func controlTextDidEndEditing(_ notification: Notification) {
            parent.isFocused = false
        }

        func control(_ control: NSControl, textView: NSTextView, doCommandBy selector: Selector) -> Bool {
            guard selector == #selector(NSResponder.insertNewline(_:)) else { return false }
            parent.onSubmit()
            return true
        }
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(self)
    }

    func makeNSView(context: Context) -> Field {
        let field = Field()
        field.isBordered = false
        field.drawsBackground = false
        field.focusRingType = .none
        field.placeholderString = "URL"
        field.font = .systemFont(ofSize: NSFont.systemFontSize)
        field.usesSingleLineMode = true
        field.lineBreakMode = .byTruncatingTail
        field.cell?.isScrollable = true
        field.setContentHuggingPriority(.defaultLow, for: .horizontal)
        field.delegate = context.coordinator
        let coordinator = context.coordinator
        field.onFocus = { coordinator.parent.isFocused = true }
        return field
    }

    func updateNSView(_ field: Field, context: Context) {
        context.coordinator.parent = self
        // only changes from outside - setting it while typing resets the selection
        if field.stringValue != text {
            field.stringValue = text
        }
    }
}
