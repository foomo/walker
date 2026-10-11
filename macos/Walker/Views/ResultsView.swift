import SwiftUI

struct ResultsView: View {
    @Bindable var model: ServerModel
    @State private var selection: ScrapeResult.ID?
    /// inspectedFolder is the prefix of the folder in the inspector
    @State private var inspectedFolder: String?
    @AppStorage("showsFolderTree") private var showsFolderTree = true
    @AppStorage("resultsSidebarWidth") private var sidebarWidth = Double(Columns.resultsSidebar.ideal)
    /// width is the width of the whole results view, inspector included
    @State private var width: CGFloat = 0

    private var selectedResult: ScrapeResult? {
        model.results.first { $0.id == selection }
    }

    private var isInspectorOpen: Bool {
        selection != nil || inspectedFolder != nil
    }

    /// hasRoomForSidebar - the inspector wins, if the sidebar does not fit next to it
    private var hasRoomForSidebar: Bool {
        !isInspectorOpen || width >= sidebarWidth + Columns.resultsTableMinWidth + Columns.inspector.min
    }

    /// showsSidebar is the sidebar setting, unless the inspector needs the room
    private var showsSidebar: Bool {
        showsFolderTree && hasRoomForSidebar
    }

    /// minWidth of everything that is visible - the window must not get narrower
    private var minWidth: CGFloat {
        Columns.resultsTableMinWidth
            + (showsSidebar ? sidebarWidth : 0)
            + (isInspectorOpen ? Columns.inspector.min : 0)
    }

    var body: some View {
        Group {
            if showsSidebar {
                // plain SwiftUI instead of HSplitView - its AppKit split view
                // disagreed with the inspector about the widths and overflowed
                HStack(spacing: 0) {
                    ResultsSidebar(
                        folders: model.folders,
                        statusOptions: model.statusOptions,
                        groupOptions: model.groupOptions,
                        bucketOptions: model.bucketOptions,
                        prefix: Binding(
                            get: { model.prefix },
                            set: { prefix in
                                // selecting a folder in the tree inspects it
                                model.prefix = prefix
                                selection = nil
                                inspectedFolder = prefix.isEmpty ? nil : prefix
                            }
                        ),
                        status: $model.status,
                        group: $model.group,
                        duration: $model.duration
                    )
                    .frame(width: sidebarWidth)
                    ResizeDivider(
                        width: $sidebarWidth,
                        range: Double(Columns.resultsSidebar.min)...Double(Columns.resultsSidebar.max)
                    )
                    resultsTable
                        .frame(minWidth: Columns.resultsTableMinWidth, maxWidth: .infinity)
                }
            } else {
                resultsTable
            }
        }
        .inspector(isPresented: Binding(
            get: { selection != nil || inspectedFolder != nil },
            set: { isPresented in
                if !isPresented {
                    selection = nil
                    inspectedFolder = nil
                }
            }
        )) {
            Group {
                if let result = selectedResult {
                    ResultInspector(result: result, lookup: model.lookupResult)
                } else if let prefix = inspectedFolder, let folder = model.folders.folder(withPrefix: prefix) {
                    FolderInspector(
                        folder: folder,
                        page: model.resultsPrefix == prefix ? model.folderPage : nil,
                        isLoading: model.resultsPrefix != prefix,
                        lookup: model.lookupResult
                    ) { subfolder in
                        model.prefix = subfolder.prefix
                    }
                }
            }
            .inspectorColumnWidth(min: Columns.inspector.min, ideal: Columns.inspector.ideal, max: Columns.inspector.max)
        }
        .frame(minWidth: minWidth)        .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { width = $0 }
        .onChange(of: selection) {
            // a result row takes over the inspector
            if selection != nil {
                inspectedFolder = nil
            }
        }
        .onChange(of: model.prefix) {
            // breadcrumbs and subfolder links move an open folder inspector along
            if inspectedFolder != nil {
                inspectedFolder = model.folders.folder(withPrefix: model.prefix) == nil ? nil : model.prefix
            }
        }
        .toolbar {
            ToolbarItem(placement: .navigation) {
                Toggle("Results Sidebar", systemImage: "list.bullet.indent", isOn: $showsFolderTree.animation())
                    .toggleStyle(.button)
                    .disabled(!hasRoomForSidebar)
                    .help(
                        !hasRoomForSidebar ? "Make the window wider or close the inspector to show status, groups and folders"
                            : showsFolderTree ? "Hide status, groups and folders (⌥⌘T)" : "Show status, groups and folders (⌥⌘T)"
                    )
                    .keyboardShortcut("t", modifiers: [.command, .option])
            }
            ToolbarItemGroup {
                Button("Reload", systemImage: "arrow.clockwise") {
                    Task { await model.loadResults() }
                }
            }
        }
        .task(id: model.resultsQuery) {
            // the search field debounces itself - everything else loads right away
            await model.loadResults()
        }
    }

    /// filterChips keep the status and group filters visible while the sidebar is hidden
    private var filterChips: [FilterChip] {
        var chips: [FilterChip] = []
        if let status = model.status {
            chips.append(FilterChip(id: "status", title: StatusClass.label(for: status), color: StatusClass.color(for: status)) {
                model.status = nil
            })
        }
        if let duration = model.duration,
           let share = model.bucketOptions.shares().first(where: { ServerModel.DurationRange(model.bucketOptions[$0.index]) == duration }) {
            chips.append(FilterChip(id: "duration", title: share.label, color: share.color) {
                model.duration = nil
            })
        }
        if let group = model.group {
            chips.append(FilterChip(id: "group", title: group.isEmpty ? "No group" : group, color: .accentColor) {
                model.group = nil
            })
        }
        return chips
    }

    private var resultsTable: some View {
        Table(model.results, selection: $selection) {
            TableColumn("Result") { result in
                ResultRow(result: result, path: result.displayPath(relativeTo: model.resultsPrefix))
            }
        }
        .tableColumnHeaders(.hidden)
        .overlay {
            if model.results.isEmpty && !model.isLoadingResults {
                if let error = model.resultsError {
                    ContentUnavailableView("Could Not Load Results", systemImage: "exclamationmark.triangle", description: Text(error))
                } else {
                    ContentUnavailableView("No Results", systemImage: "doc.text.magnifyingglass")
                }
            }
        }
        .safeAreaInset(edge: .top, spacing: 0) {
            // the prefix is a folder, if it is one of the tree - like on the server
            let folder = model.folders.folder(withPrefix: model.prefix)
            let crumbs = folder == nil ? [] : Crumb.crumbs(for: model.prefix)
            let chips = filterChips
            VStack(spacing: 0) {
                SearchField(text: $model.search, prompt: "Search paths, e.g. bestseller/acme")
                    .padding(.horizontal)
                    .padding(.vertical, 8)
                if !crumbs.isEmpty || !chips.isEmpty {
                    Breadcrumbs(crumbs: crumbs, folder: folder, chips: chips, prefix: $model.prefix)
                        .padding(.top, -4)
                }
            }
            .background(.bar)
        }
        .safeAreaInset(edge: .bottom) {
            pager
        }
    }

    private var pager: some View {
        HStack {
            if model.isLoadingResults {
                ProgressView()
                    .controlSize(.small)
            }
            Spacer()
            Button("Previous Page", systemImage: "chevron.left") { model.page -= 1 }
                .disabled(model.page <= 0)
            Text("Page \(min(model.page + 1, max(model.numPages, 1))) of \(max(model.numPages, 1))")
                .monospacedDigit()
            Button("Next Page", systemImage: "chevron.right") { model.page += 1 }
                .disabled(model.page + 1 >= model.numPages)
        }
        .labelStyle(.iconOnly)
        .padding(.horizontal)
        .padding(.vertical, 6)
        .background(.bar)
    }
}

/// ResultRow shows a result in two columns - status and response time on
/// the left, path, group and title on the right - so that the table stays narrow.
struct ResultRow: View {
    let result: ScrapeResult
    let path: String

    /// statusWidth fits "failed" and "10.5 s"
    private static let statusWidth: CGFloat = 56

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                Text(StatusClass.label(for: result.code))
                    .monospacedDigit()
                    .fontWeight(.semibold)
                    .foregroundStyle(StatusClass.color(for: result.code))
                Text(result.duration.formatted)
                    .font(.caption)
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
            }
            .lineLimit(1)
            .frame(width: Self.statusWidth, alignment: .leading)

            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: path)
                    .truncationMode(.middle)
                    .help(result.targetURL)
                if !result.group.isEmpty {
                    Text(result.group)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Text(verbatim: result.structure.title.isEmpty ? "–" : result.structure.title)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .truncationMode(.tail)
            }
            .lineLimit(1)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, 2)
    }
}

/// ResizeDivider is a divider, that resizes the column to its left by dragging.
struct ResizeDivider: View {
    @Binding var width: Double
    let range: ClosedRange<Double>

    @State private var startWidth: Double?

    var body: some View {
        Divider()
            .overlay {
                // a wider, invisible grip than the hairline
                Color.clear
                    .frame(width: 8)
                    .contentShape(.rect)
                    .pointerStyle(.columnResize)
                    .gesture(
                        DragGesture(minimumDistance: 1, coordinateSpace: .global)
                            .onChanged { value in
                                let start = startWidth ?? width
                                startWidth = start
                                width = min(max(start + value.translation.width, range.lowerBound), range.upperBound)
                            }
                            .onEnded { _ in
                                startWidth = nil
                            }
                    )
            }
    }
}

/// SearchField is a search field for the top of a list - .searchable only
/// goes into the toolbar or the sidebar on macOS. ⌘F focuses it. Typing is
/// applied to text after 300ms without a key stroke, enter applies it at once.
struct SearchField: View {
    @Binding var text: String
    let prompt: String

    /// draft is what is being typed - text follows it debounced
    @State private var draft = ""
    @FocusState private var isFocused: Bool

    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.secondary)
            TextField("Search", text: $draft, prompt: Text(verbatim: prompt))
                .textFieldStyle(.plain)
                .labelsHidden()
                .focused($isFocused)
                .onSubmit { text = draft }
                .onExitCommand { isFocused = false }
            if !draft.isEmpty {
                Button {
                    draft = ""
                    text = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .help("Clear the search")
            }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 5)
        .background(.background, in: .rect(cornerRadius: 7))
        .overlay {
            RoundedRectangle(cornerRadius: 7)
                .strokeBorder(isFocused ? Color.accentColor : Color.secondary.opacity(0.3), lineWidth: isFocused ? 2 : 1)
        }
        .background {
            // ⌘F like .searchable
            Button("Find") { isFocused = true }
                .keyboardShortcut("f")
                .hidden()
        }
        .task(id: draft) {
            guard draft != text else { return }
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            text = draft
        }
        .onAppear { draft = text }
        .onChange(of: text) {
            // changed from outside, e.g. cleared
            if text != draft {
                draft = text
            }
        }
    }
}
