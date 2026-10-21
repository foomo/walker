import SwiftUI

struct ServerDetailView: View {
    @Bindable var model: ServerModel
    @AppStorage(SettingsView.refreshIntervalKey) private var refreshInterval = SettingsView
        .defaultRefreshInterval
    @State private var section = Section.overview
    @Environment(\.openWindow) private var openWindow

    enum Section: String, CaseIterable, Identifiable {
        case overview = "Overview"
        case results = "Results"
        case brokenLinks = "Broken Links"
        case redirects = "Redirects"
        case links = "Links"
        case performance = "Performance"

        var id: Self { self }

        /// tabs of the section switch - the overview first, it opens from the server name too
        static let tabs = allCases
    }

    var body: some View {
        // the status bar sits below the sections, not over them - split views
        // and the inspector do not pass a safe area inset on, so it would
        // cover their last rows and the pager of the results
        VStack(spacing: 0) {
            sections
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            StatusBar(current: model.current, complete: model.complete, connection: model.connection)
        }
    }

    @ViewBuilder
    private var sections: some View {
        Group {
            switch section {
            case .overview:
                OverviewView(model: model)
            case .results:
                ResultsView(model: model)
            case .brokenLinks:
                BrokenLinksView(model: model)
            case .redirects:
                RedirectsView(model: model)
            case .links:
                LinksView(model: model)
            case .performance:
                PerformanceView(model: model)
            }
        }
        // the window keeps its title, the toolbar shows the server in its group
        .navigationTitle(model.server.name)
        .navigationSubtitle(model.server.baseURL.absoluteString)
        .toolbar(removing: .title)
        .toolbar {
            // the server and the scrape all sections show, side by side - the
            // server is a label, not a control, so it gets no glass background
            ToolbarItem(placement: .navigation) {
                Button {
                    section = .overview
                } label: {
                    VStack(alignment: .leading, spacing: 0) {
                        Text(model.server.name)
                            .font(.headline)
                        Text(model.server.baseURL.absoluteString)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .lineLimit(1)
                    .padding(.horizontal, 6)
                    .contentShape(.rect)
                }
                .buttonStyle(.plain)
                .pointerStyle(.link)
                .help("Overview of \(model.server.name)")
            }
            .sharedBackgroundVisibility(.hidden)
            // the overview shows both scrapes side by side
            if section != .overview {
                ToolbarItem(placement: .navigation) {
                    Picker("Scrape", selection: $model.scrape) {
                        ForEach(ServerModel.Scrape.allCases) { scrape in
                            Text(scrape.title)
                                .tag(scrape)
                                .disabled(!model.isAvailable(scrape))
                        }
                    }
                    .pickerStyle(.segmented)
                    .labelsHidden()
                    .help(model.isAvailable(.complete) ? "" : "No scrape has completed yet")
                }
            }
            // a local walker is started and stopped from here, its config and log are in the overview
            if model.server.isLocal {
                ToolbarItem(placement: .primaryAction) {
                    LocalWalkerButton(server: model.server)
                }
            }
            ToolbarItem(placement: .primaryAction) {
                Button("Schema Developer", systemImage: "checklist") {
                    openWindow(id: SchemaDevWindow.id, value: model.server.id)
                }
                .keyboardShortcut("s", modifiers: [.command, .shift])
                .help("Iterate on an html schema against a page of this server's site")
            }
            ToolbarItem(placement: .principal) {
                Picker("Section", selection: $section) {
                    ForEach(Section.tabs) { section in
                        Text(section.rawValue).tag(section)
                    }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
            }
        }
        .onChange(of: model.isAvailable(.complete)) {
            // e.g. walker restarted - there is nothing complete to look at
            if !model.isAvailable(model.scrape) {
                model.scrape = .current
            }
        }
        .onChange(of: model.scrape) {
            // in case a disabled segment can be clicked after all
            if !model.isAvailable(model.scrape) {
                model.scrape = .current
            }
        }
        .task(id: refreshInterval) {
            await model.pollScrapes(every: .seconds(max(refreshInterval, 1)))
        }
    }
}
