import AppKit
import SwiftUI

/// SchemaDevWindow is the content of a schema developer window - one per server
struct SchemaDevWindow: View {
    static let id = "schema-developer"

    let serverID: Server.ID?
    @Environment(ServerStore.self) private var store

    var body: some View {
        if let server = store.servers.first(where: { $0.id == serverID }) {
            SchemaDevView(server: server)
        } else {
            ContentUnavailableView("No Server", systemImage: "server.rack", description: Text("The server of this window was removed."))
        }
    }
}

/// SchemaDevView iterates on a schema and a website: the evaluated schema as a
/// tree next to the page, a selected node is highlighted in the page
struct SchemaDevView: View {
    @Environment(ServerStore.self) private var store
    @State private var model: SchemaDevModel
    /// showsPageData shows the head and the JSON-LD between the schema and the page
    @AppStorage("schemaDevShowsPageData") private var showsPageData = true

    init(server: Server) {
        _model = State(initialValue: SchemaDevModel(server: server))
    }

    var body: some View {
        // every column takes the full height - small content like a lint
        // error would otherwise be centered vertically by the split view
        HSplitView {
            SchemaPane(model: model)
                .frame(minWidth: 320, idealWidth: 440, maxWidth: 700, maxHeight: .infinity, alignment: .top)
            if showsPageData {
                PageDataPane(model: model)
                    .frame(minWidth: 280, idealWidth: 380, maxWidth: 700, maxHeight: .infinity, alignment: .top)
            }
            page
                .frame(minWidth: 420, maxWidth: .infinity, maxHeight: .infinity)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .navigationTitle("Schema – \(model.server.name)")
        .toolbar { toolbar }
        .task(id: model.schemaRoot) {
            await model.loadRoot()
            // edits to the schema files validate again, until the root changes
            await model.watchSchema()
        }
        .onChange(of: model.group) {
            Task { await model.validate() }
        }
        .onChange(of: model.source) {
            Task { await model.validate() }
        }
        .onChange(of: model.schemaRoot) {
            Task { await model.validate() }
        }
        .onAppear {
            model.open()
        }
        // the window of a local config starts with the site and schema root of the config
        .task {
            guard model.server.isLocal, model.address.isEmpty, model.schemaRoot.isEmpty else { return }
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
            SchemaWebView(
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
            .help("Reload the page and the schema, then validate again")
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
                ? "Add a local config to load its site and schema root"
                : "Open the site of a local config and use its schema root")
        }
        ToolbarItem {
            SchemaRootMenu(model: model)
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
            .help("The group schema to validate with")
        }
        ToolbarItem {
            Toggle(isOn: $showsPageData) {
                Label("Head & JSON-LD", systemImage: "curlybraces.square")
            }
            .help(showsPageData ? "Hide the head and the JSON-LD of the validated html" : "Show the head and the JSON-LD of the validated html")
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
                ForEach(SchemaDevModel.Source.allCases) { source in
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

/// SchemaRootMenu picks the schema root - a directory on the server, which
/// shares the file system with this app
private struct SchemaRootMenu: View {
    @Bindable var model: SchemaDevModel

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
            Label(model.schemaRoot.isEmpty ? "Schema Root" : (model.schemaRoot as NSString).lastPathComponent, systemImage: "folder")
                .labelStyle(.titleAndIcon)
        }
        .help(model.schemaRoot.isEmpty ? "Choose the schema root" : model.schemaRoot)
    }

    private func choose() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.message = "Choose a schema root - the directory with default.html and groups/"
        if !model.schemaRoot.isEmpty {
            panel.directoryURL = URL(filePath: model.schemaRoot)
        }
        if panel.runModal() == .OK, let url = panel.url {
            model.chooseRoot(url.path(percentEncoded: false))
        }
    }
}

/// SchemaPane shows the result of the validation: summary, lint errors or the schema tree
private struct SchemaPane: View {
    @Bindable var model: SchemaDevModel

    var body: some View {
        VStack(spacing: 0) {
            SchemaSummary(model: model)
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
                    SourceLink(root: model.schemaRoot, source: "\(lintError.file):\(lintError.line)")
                        .font(.caption.monospaced())
                    Text(lintError.message)
                }
            }
        } else if model.schemaRoot.isEmpty {
            ContentUnavailableView("No Schema Root", systemImage: "folder", description: Text("Choose the schema root in the toolbar."))
        } else if model.items.isEmpty {
            ContentUnavailableView("No Validation", systemImage: "checklist", description: Text(model.isValidating ? "Validating…" : "Load a page to validate it."))
        } else {
            SchemaTreeView(items: model.items, selection: $model.selection, collapsed: Binding(
                get: { model.collapsed ?? [] },
                set: { model.collapsed = $0 }
            ))
            .environment(\.schemaRoot, model.schemaRoot)
        }
    }
}

/// SchemaSummary is the head of the schema pane: score, penalty and findings
private struct SchemaSummary: View {
    let model: SchemaDevModel

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
                Text(report.schema + sourceDescription + reloadDescription)
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
        guard let reloaded = model.schemaReloadedAt else { return "" }
        return " · schema reloaded " + reloaded.formatted(date: .omitted, time: .standard)
    }

    private var sourceDescription: String {
        guard let validation = model.validation else { return "" }
        if validation.source == "server" {
            return " · fetched by the server, status \(validation.statusCode)"
        }
        return " · rendered DOM"
    }

    private func findingCount(_ report: HTMLSchemaVO.Report, _ severity: HTMLSchemaVO.Severity) -> some View {
        let count = (report.findings ?? []).filter { $0?.severity == severity }.count
        return Text("\(count) \(severity.rawValue)")
            .foregroundStyle(count > 0 ? HTMLSchemaVO.TraceStatus(rawValue: severity.rawValue).color : Color.secondary)
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
