import SwiftUI

/// BrokenLinksView lists the broken pages of the selected scrape - the most
/// linked first - and where they are linked from.
struct BrokenLinksView: View {
    let model: ServerModel
    @State private var selection: BrokenLink.ID?

    private var selected: BrokenLink? {
        model.brokenLinks.first { $0.id == selection }
    }

    /// reload, when the scrape is switched or another scrape completed
    private struct Query: Hashable {
        let scrape: ServerModel.Scrape
        let completed: Date?
    }

    var body: some View {
        Table(model.brokenLinks, selection: $selection) {
            TableColumn("Broken Link") { link in
                BrokenLinkRow(link: link)
            }
        }
        .tableColumnHeaders(.hidden)
        .overlay {
            if model.brokenLinks.isEmpty && !model.isLoadingBrokenLinks {
                if let error = model.brokenLinksError {
                    ContentUnavailableView("Could Not Load Broken Links", systemImage: "exclamationmark.triangle", description: Text(error))
                } else {
                    ContentUnavailableView("No Broken Links", systemImage: "checkmark.seal", description: Text("No page of this scrape answered with an error."))
                }
            }
        }
        .safeAreaInset(edge: .bottom) {
            HStack {
                if model.isLoadingBrokenLinks {
                    ProgressView()
                        .controlSize(.small)
                }
                Spacer()
                Text("\(model.brokenLinks.count) broken pages")
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
            }
            .padding(.horizontal)
            .padding(.vertical, 6)
            .background(.bar)
        }
        .inspector(isPresented: Binding(get: { selection != nil }, set: { if !$0 { selection = nil } })) {
            Group {
                if let selected {
                    BrokenLinkInspector(link: selected, lookup: model.lookupResult)
                        .id(selected.id)
                }
            }
            .inspectorColumnWidth(min: Columns.inspector.min, ideal: Columns.inspector.ideal, max: Columns.inspector.max)
        }
        .frame(minWidth: Columns.resultsTableMinWidth + (selection != nil ? Columns.inspector.min : 0))
        .toolbar {
            ToolbarItem {
                Button("Reload", systemImage: "arrow.clockwise") {
                    Task { await model.loadBrokenLinks() }
                }
            }
        }
        .task(id: Query(scrape: model.scrape, completed: model.complete?.completed)) {
            await model.loadBrokenLinks()
        }
    }
}

extension BrokenLink: Identifiable {
    public var id: String { targetURL }
}

/// BrokenLinkRow - status and the number of linking pages on the left, path,
/// group and the error on the right, like the result rows.
struct BrokenLinkRow: View {
    let link: BrokenLink

    private var linkedFrom: Int { link.linkedFrom?.count ?? 0 }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                Text(StatusClass.label(for: link.code))
                    .monospacedDigit()
                    .fontWeight(.semibold)
                    .foregroundStyle(StatusClass.color(for: link.code))
                Label(String(linkedFrom), systemImage: "link")
                    .font(.caption)
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
                    .help("Linked from \(linkedFrom) pages, \(link.links) links")
            }
            .lineLimit(1)
            .frame(width: 56, alignment: .leading)

            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: link.targetURL.urlPath)
                    .truncationMode(.middle)
                    .help(link.targetURL)
                if !link.group.isEmpty {
                    Text(link.group)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Text(linkedFrom == 0 ? "not linked from any scraped page" : "linked from \(linkedFrom) pages")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .lineLimit(1)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, 2)
    }
}

/// BrokenLinkInspector shows a broken page and the pages linking to it - a
/// linking page opens below with its preview, the broken links outlined.
struct BrokenLinkInspector: View {
    let link: BrokenLink
    let lookup: ResultLookup

    @State private var source: String?
    @State private var sourcePage = SourcePage.none

    enum SourcePage {
        case none
        case loading
        case loaded(ScrapeResult)
        case missing(String)
    }

    var body: some View {
        Form {
            Section("Broken page") {
                LabeledContent("URL") {
                    URLValue(link.targetURL, isLink: true)
                }
                LabeledContent("Status") {
                    Text(link.status.isEmpty ? StatusClass.label(for: link.code) : link.status)
                        .foregroundStyle(StatusClass.color(for: link.code))
                }
                if !link.error.isEmpty {
                    LabeledContent("Error") {
                        Text(link.error)
                            .foregroundStyle(.red)
                    }
                }
                LabeledContent("Group", value: link.group.isEmpty ? "–" : link.group)
                LabeledContent("Linked from", value: "\(link.linkedFrom?.count ?? 0) pages, \(link.links) links")
            }

            Section("Linked from") {
                if (link.linkedFrom ?? []).isEmpty {
                    Text("No scraped page links here - it may be linked from outside, or was a start path.")
                        .foregroundStyle(.secondary)
                }
                ForEach(link.linkedFrom ?? [], id: \.targetURL) { linkSource in
                    Button {
                        source = linkSource.targetURL
                    } label: {
                        HStack {
                            Image(systemName: source == linkSource.targetURL ? "eye.fill" : "doc")
                                .foregroundStyle(source == linkSource.targetURL ? Color.accentColor : .secondary)
                            Text(verbatim: linkSource.targetURL.urlPath)
                                .lineLimit(1)
                                .truncationMode(.middle)
                            Spacer()
                            Text("\(linkSource.count)×")
                                .monospacedDigit()
                                .foregroundStyle(.secondary)
                        }
                        .contentShape(.rect)
                    }
                    .buttonStyle(.plain)
                    .help("Show \(linkSource.targetURL) with the broken link outlined")
                }
            }

            switch sourcePage {
            case .none:
                EmptyView()
            case .loading:
                Section("Linking page") {
                    ProgressView()
                        .controlSize(.small)
                        .frame(maxWidth: .infinity)
                }
            case let .loaded(page):
                BrowsableResult(start: page, lookup: lookup, highlight: link.targetURL)
                    .id(page.targetURL)
            case let .missing(url):
                Section("Linking page") {
                    Label("\(url) is not part of the selected scrape anymore.", systemImage: "questionmark.circle")
                        .foregroundStyle(.secondary)
                }
            }
        }
        .formStyle(.grouped)
        .textSelection(.enabled)
        .task(id: source) {
            guard let source else { return }
            sourcePage = .loading
            do {
                let page = try await lookup(source)
                guard !Task.isCancelled else { return }
                sourcePage = page.map { .loaded($0) } ?? .missing(source)
            } catch is CancellationError {
                return
            } catch {
                sourcePage = .missing(source)
            }
        }
    }
}
