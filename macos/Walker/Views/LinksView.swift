import SwiftUI

/// LinksView shows the internal link structure of the selected scrape: how
/// often every page is linked - the least linked end shows the orphans.
struct LinksView: View {
    let model: ServerModel
    @State private var selection: WalkerVO.PageLinks.ID?
    @AppStorage("linksOrder") private var order = Order.mostLinked

    enum Order: String, CaseIterable, Identifiable {
        case mostLinked = "Most linked"
        case leastLinked = "Least linked"

        var id: Self { self }
    }

    private var sorted: [WalkerVO.PageLinks] {
        model.links.sorted { a, b in
            if a.linkedFrom != b.linkedFrom {
                return order == .mostLinked ? a.linkedFrom > b.linkedFrom : a.linkedFrom < b.linkedFrom
            }
            return a.targetURL < b.targetURL
        }
    }

    private var orphans: Int {
        model.links.filter { $0.linkedFrom == 0 }.count
    }

    /// reload, when the scrape is switched or another scrape completed
    private struct Query: Hashable {
        let scrape: ServerModel.Scrape
        let completed: Date?
    }

    var body: some View {
        Table(sorted, selection: $selection) {
            TableColumn("Page") { page in
                PageLinksRow(page: page)
            }
        }
        .tableColumnHeaders(.hidden)
        .overlay {
            if model.links.isEmpty && !model.isLoadingLinks {
                if let error = model.linksError {
                    ContentUnavailableView("Could Not Load Links", systemImage: "exclamationmark.triangle", description: Text(error))
                } else {
                    ContentUnavailableView("No Pages", systemImage: "link", description: Text("This scrape has no pages yet."))
                }
            }
        }
        .safeAreaInset(edge: .bottom) {
            HStack {
                Picker("Order", selection: $order) {
                    ForEach(Order.allCases) { order in
                        Text(order.rawValue).tag(order)
                    }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .fixedSize()
                if model.isLoadingLinks {
                    ProgressView()
                        .controlSize(.small)
                }
                Spacer()
                Text("\(model.links.count) pages · \(orphans) not linked")
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
            }
            .padding(.horizontal)
            .padding(.vertical, 6)
            .background(.bar)
        }
        .inspector(isPresented: Binding(get: { selection != nil }, set: { if !$0 { selection = nil } })) {
            Group {
                if let selection {
                    PageLinksInspector(url: selection, model: model)
                        .id(selection)
                }
            }
            .inspectorColumnWidth(min: Columns.inspector.min, ideal: Columns.inspector.ideal, max: Columns.inspector.max)
        }
        .frame(minWidth: Columns.resultsTableMinWidth + (selection != nil ? Columns.inspector.min : 0))
        .toolbar {
            ToolbarItem {
                Button("Reload", systemImage: "arrow.clockwise") {
                    Task { await model.loadLinks() }
                }
            }
        }
        .task(id: Query(scrape: model.scrape, completed: model.complete?.completed)) {
            await model.loadLinks()
        }
    }
}

extension WalkerVO.PageLinks: Identifiable {
    public var id: String { targetURL }
}

/// PageLinksRow - linking pages and the status on the left, path, title and
/// the link counts on the right.
struct PageLinksRow: View {
    let page: WalkerVO.PageLinks

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                Label(String(page.linkedFrom), systemImage: page.linkedFrom == 0 ? "link.badge.plus" : "link")
                    .monospacedDigit()
                    .fontWeight(.semibold)
                    .foregroundStyle(page.linkedFrom == 0 ? Color.orange : Color.primary)
                    .help(page.linkedFrom == 0 ? "No scraped page links here" : "Linked from \(page.linkedFrom) pages")
                Text(StatusClass.label(for: page.code))
                    .font(.caption)
                    .monospacedDigit()
                    .foregroundStyle(StatusClass.color(for: page.code))
            }
            .lineLimit(1)
            .frame(width: 64, alignment: .leading)

            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: page.targetURL.urlPath)
                    .truncationMode(.middle)
                    .help(page.targetURL)
                Text(verbatim: page.title.isEmpty ? "–" : page.title)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text("\(page.links) links in · \(page.linksTo) links out\(page.group.isEmpty ? "" : " · \(page.group)")")
                    .font(.caption)
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
            }
            .lineLimit(1)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, 2)
    }
}

/// PageLinksInspector shows a page, the pages linking to it - with the
/// preview and the links outlined - and the links of the page itself.
struct PageLinksInspector: View {
    let url: String
    let model: ServerModel

    @State private var page: WalkerVO.ScrapeResult?
    @State private var linkedFrom: WalkerVO.LinkedFromPage?
    @State private var limit = 20
    @State private var error: String?

    var body: some View {
        Form {
            Section {
                ResultHeader(title: page?.structure.title ?? "", description: page?.structure.description ?? "", url: url)
            }
            if let error {
                Section {
                    Label(error, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.red)
                }
            } else if let linkedFrom {
                LinkingPages(
                    target: url,
                    sources: linkedFrom.sources ?? [],
                    lookup: model.lookupResult,
                    total: linkedFrom.total,
                    groups: linkedFrom.groups ?? []
                ) {
                    limit += 50
                }
            } else {
                Section("Linked from") {
                    ProgressView()
                        .controlSize(.small)
                        .frame(maxWidth: .infinity)
                }
            }
            if let page {
                Section("Links to") {
                    LinksDisclosure(links: page.normalizedLinks ?? page.links ?? [:])
                }
            }
        }
        .formStyle(.grouped)
        .textSelection(.enabled)
        .task {
            do {
                page = try await model.lookupResult(url)
            } catch {}
        }
        .task(id: limit) {
            do {
                let linkedFrom = try await model.linkedFrom(url, limit: limit)
                guard !Task.isCancelled else { return }
                self.linkedFrom = linkedFrom
            } catch is CancellationError {
                return
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}
