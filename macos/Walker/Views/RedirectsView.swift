import SwiftUI

/// RedirectsView lists the redirecting urls of the selected scrape - the
/// longest chains first - with their hops and where they are linked from.
struct RedirectsView: View {
    let model: ServerModel
    @State private var selection: RedirectChain.ID?

    private var selected: RedirectChain? {
        model.redirects.first { $0.id == selection }
    }

    /// reload, when the scrape is switched or another scrape completed
    private struct Query: Hashable {
        let scrape: ServerModel.Scrape
        let completed: Date?
    }

    var body: some View {
        Table(model.redirects, selection: $selection) {
            TableColumn("Redirect") { chain in
                RedirectRow(chain: chain)
            }
        }
        .tableColumnHeaders(.hidden)
        .overlay {
            if model.redirects.isEmpty && !model.isLoadingRedirects {
                if let error = model.redirectsError {
                    ContentUnavailableView("Could Not Load Redirects", systemImage: "exclamationmark.triangle", description: Text(error))
                } else {
                    ContentUnavailableView("No Redirects", systemImage: "checkmark.seal", description: Text("No url of this scrape redirected."))
                }
            }
        }
        .safeAreaInset(edge: .bottom) {
            HStack {
                if model.isLoadingRedirects {
                    ProgressView()
                        .controlSize(.small)
                }
                Spacer()
                Text("\(model.redirects.count) redirects")
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
                    RedirectInspector(chain: selected, lookup: model.lookupResult)
                        .id(selected.id)
                }
            }
            .inspectorColumnWidth(min: Columns.inspector.min, ideal: Columns.inspector.ideal, max: Columns.inspector.max)
        }
        .frame(minWidth: Columns.resultsTableMinWidth + (selection != nil ? Columns.inspector.min : 0))
        .toolbar {
            ToolbarItem {
                Button("Reload", systemImage: "arrow.clockwise") {
                    Task { await model.loadRedirects() }
                }
            }
        }
        .task(id: Query(scrape: model.scrape, completed: model.complete?.completed)) {
            await model.loadRedirects()
        }
    }
}

extension RedirectChain: Identifiable {
    public var id: String { targetURL }

    /// hopCodes are the codes of the hops, e.g. "301 → 302"
    var hopCodes: String {
        (hops ?? []).map { String($0.code) }.joined(separator: " → ")
    }
}

/// RedirectRow - the hop codes and the final status on the left, the source
/// path, where it ends and the group on the right.
struct RedirectRow: View {
    let chain: RedirectChain

    private var hops: Int { chain.hops?.count ?? 0 }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                Text(chain.hopCodes)
                    .monospacedDigit()
                    .fontWeight(.semibold)
                    .foregroundStyle(chain.permanent ? StatusClass.color(for: 301) : .orange)
                    .help(chain.permanent ? "permanent" : "contains temporary redirects")
                Text("→ \(StatusClass.label(for: chain.finalCode))")
                    .font(.caption)
                    .monospacedDigit()
                    .foregroundStyle(StatusClass.color(for: chain.finalCode))
                    .help("the chain ends with \(StatusClass.label(for: chain.finalCode))")
            }
            .lineLimit(1)
            .frame(width: 72, alignment: .leading)

            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: chain.targetURL.urlPath)
                    .truncationMode(.middle)
                    .help(chain.targetURL)
                Text(verbatim: "→ " + chain.finalURL.urlPath)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .truncationMode(.middle)
                    .help(chain.finalURL)
                HStack(spacing: 8) {
                    if hops > 1 {
                        Label("\(hops) hops", systemImage: "arrow.triangle.branch")
                            .foregroundStyle(.orange)
                    }
                    if !chain.group.isEmpty {
                        Text(chain.group)
                    }
                    Label(String(chain.linkedFrom?.count ?? 0), systemImage: "link")
                        .monospacedDigit()
                        .help("Linked from \(chain.linkedFrom?.count ?? 0) pages, \(chain.links) links")
                }
                .font(.caption)
                .foregroundStyle(.secondary)
            }
            .lineLimit(1)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, 2)
    }
}

/// RedirectInspector shows the hops of a redirect and the pages linking to the
/// redirecting url - they should link to where it ends instead.
struct RedirectInspector: View {
    let chain: RedirectChain
    let lookup: ResultLookup

    var body: some View {
        Form {
            Section("Chain") {
                hop(code: nil, url: chain.targetURL)
                ForEach(Array((chain.hops ?? []).enumerated()), id: \.offset) { _, redirect in
                    hop(code: redirect.code, url: redirect.url)
                }
                LabeledContent("Ends with") {
                    Text(StatusClass.label(for: chain.finalCode))
                        .foregroundStyle(StatusClass.color(for: chain.finalCode))
                }
                if !chain.permanent {
                    Label("Contains temporary redirects - search engines keep the old url.", systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.orange)
                }
                if (chain.hops?.count ?? 0) > 1 {
                    Label("\(chain.hops?.count ?? 0) hops - link to the final url directly.", systemImage: "arrow.triangle.branch")
                        .foregroundStyle(.orange)
                }
                LabeledContent("Group", value: chain.group.isEmpty ? "–" : chain.group)
            }
            LinkingPages(target: chain.targetURL, sources: chain.linkedFrom ?? [], lookup: lookup)
        }
        .formStyle(.grouped)
        .textSelection(.enabled)
    }

    /// hop - the start url, or the code of the response and where it points
    private func hop(code: Int?, url: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Group {
                if let code {
                    Text("↳ \(code)")
                        .foregroundStyle(code == 301 || code == 308 ? StatusClass.color(for: code) : .orange)
                } else {
                    Text("start")
                        .foregroundStyle(.secondary)
                }
            }
            .font(.callout.monospacedDigit().weight(.semibold))
            .frame(width: 56, alignment: .leading)
            URLValue(url, isLink: true)
            Spacer(minLength: 0)
        }
    }
}
