import AppKit
import SwiftUI

/// ShapeDevWindow is the content of a shape developer window - one per server
struct ShapeDevWindow: View {
    static let id = "shape-developer"

    let serverID: Server.ID?
    @Environment(ServerStore.self) private var store

    var body: some View {
        if let server = store.servers.first(where: { $0.id == serverID }) {
            ShapeDevView(server: server)
        } else {
            ContentUnavailableView("No Server", systemImage: "server.rack", description: Text("The server of this window was removed."))
        }
    }
}

/// ShapeDevView iterates on a shape and a website: the evaluated shape as a
/// tree next to the page, a selected node is highlighted in the page
struct ShapeDevView: View {
    @Environment(ServerStore.self) private var store
    @State private var model: ShapeDevModel
    /// showsPageData shows the head and the JSON-LD between the shape and the page
    @AppStorage("shapeDevShowsPageData") private var showsPageData = true
    /// showsOutline shows the semantic outline of the page right of it
    @AppStorage("shapeDevShowsOutline") private var showsOutline = false

    init(server: Server) {
        _model = State(initialValue: ShapeDevModel(server: server))
    }

    var body: some View {
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
        .navigationTitle("Shape – \(model.server.name)")
        .toolbar { toolbar }
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
        .onAppear {
            model.open()
        }
        // the window of a local config starts with the site and shape root of the config
        .task {
            guard model.server.isLocal, model.address.isEmpty, model.shapeRoot.isEmpty else { return }
            await load(model.server)
        }
    }

    private func load(_ server: Server) async {
        guard let url = server.configURL, let result = try? await store.refreshLocal(server.id) else { return }
        model.load(result.form, configURL: url)
    }

    @ViewBuilder
    private var page: some View {
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

    @ToolbarContentBuilder
    private var toolbar: some ToolbarContent {
        ToolbarItem(placement: .navigation) {
            Button("Reload", systemImage: "arrow.clockwise") {
                model.reload()
            }
            .keyboardShortcut("r")
            .help("Reload the page and the shape, then validate again")
        }
        ToolbarItem(placement: .principal) {
            AddressField(text: $model.address) { model.open() }
                .frame(minWidth: 280, idealWidth: 480)
        }
        .sharedBackgroundVisibility(.hidden)
        ToolbarItem {
            Menu("Load Config", systemImage: "doc.text") {
                ForEach(store.localServers) { server in
                    Button(server.name) {
                        Task { await load(server) }
                    }
                    .help(server.configPath ?? "")
                }
            }
            .disabled(store.localServers.isEmpty)
            .help(store.localServers.isEmpty
                ? "Add a local config to load its site and shape root"
                : "Open the site of a local config and use its shape root")
        }
        ToolbarItem {
            ShapeRootMenu(model: model)
        }
        ToolbarItem {
            EditorMenu()
        }
        ToolbarItem {
            Picker("Group", selection: $model.group) {
                Text(autoGroupTitle).tag("")
                if let groups = model.rootInfo?.groups, !groups.isEmpty {
                    Divider()
                    ForEach(groups, id: \.self) { group in
                        Text(group).tag(group)
                    }
                }
            }
            .frame(minWidth: 160)
            .help("The group shape to validate with")
        }
        ToolbarItem {
            Toggle(isOn: $showsPageData) {
                Label("Head & JSON-LD", systemImage: "curlybraces.square")
            }
            .help(showsPageData ? "Hide the head and the JSON-LD of the validated html" : "Show the head and the JSON-LD of the validated html")
        }
        ToolbarItem {
            Toggle(isOn: $showsOutline) {
                Label("Document Outline", systemImage: "list.bullet.indent")
            }
            .help(showsOutline ? "Hide the outline of the page" : "Show the outline of the page: landmarks, articles, sections, headings - and its content")
        }
        ToolbarItem {
            Button("Web Inspector", systemImage: "hammer") {
                model.highlighter.showInspector()
            }
            .keyboardShortcut("i", modifiers: [.command, .option])
            .disabled(model.pageURL == nil)
            .help("Open the Web Inspector of the page (⌥⌘I) - or right click an element and choose Inspect Element")
        }
        ToolbarItem {
            Picker("Source", selection: $model.source) {
                ForEach(ShapeDevModel.Source.allCases) { source in
                    Text(source.rawValue).tag(source)
                }
            }
            .pickerStyle(.segmented)
            .help("Server: the server fetches the page like the crawler - Rendered: a snapshot of the DOM in the web view")
        }
    }

    /// autoGroupTitle names the group auto resolves to
    private var autoGroupTitle: String {
        if let detected = model.validation?.detectedGroup, !detected.isEmpty {
            return "Auto (\(detected))"
        }
        if let header = model.rootInfo?.groupHeader, !header.isEmpty {
            return "Auto (header \(header))"
        }
        return "Auto"
    }
}

/// ShapeRootMenu picks the shape root - a directory on the server, which
/// shares the file system with this app
private struct ShapeRootMenu: View {
    @Bindable var model: ShapeDevModel

    var body: some View {
        Menu {
            ForEach(model.recentRoots, id: \.self) { root in
                Button(root) { model.chooseRoot(root) }
            }
            if !model.recentRoots.isEmpty {
                Divider()
            }
            Button("Choose…") { choose() }
        } label: {
            Label(model.shapeRoot.isEmpty ? "Shape Root" : (model.shapeRoot as NSString).lastPathComponent, systemImage: "folder")
                .labelStyle(.titleAndIcon)
        }
        .help(model.shapeRoot.isEmpty ? "Choose the shape root" : model.shapeRoot)
    }

    private func choose() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.message = "Choose a shape root - the directory with default.semanticshape.html and groups/"
        if !model.shapeRoot.isEmpty {
            panel.directoryURL = URL(filePath: model.shapeRoot)
        }
        if panel.runModal() == .OK, let url = panel.url {
            model.chooseRoot(url.path(percentEncoded: false))
        }
    }
}

/// OutlinePane shows the semantic outline of the page, from the same source as
/// the validation - hovering an element highlights it in the page
private struct OutlinePane: View {
    let model: ShapeDevModel
    /// showsContent adds texts, links, images and figures - shared with the result inspector
    @AppStorage("outlineShowsContent") private var showsContent = false

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            content
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        }
        // a new page, source or content setting loads again - shape edits don't change the page
        .task(id: "\(model.pageVersion) \(model.source.rawValue) \(showsContent)") {
            await model.loadOutline(content: showsContent)
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
            Toggle("Content", isOn: $showsContent)
                .toggleStyle(.switch)
                .controlSize(.mini)
                .help(showsContent ? "Show the structure only" : "Show the texts, links, images and figures too")
        }
        .padding(10)
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
                    SemanticDocumentContent(result: result, pageURL: model.highlighter.pageURL ?? model.pageURL) { selector, _ in
                        model.hoverOutline(selector)
                    }
                }
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .textSelection(.enabled)
            }
        }
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
