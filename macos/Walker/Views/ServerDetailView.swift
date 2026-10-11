import SwiftUI

struct ServerDetailView: View {
    @Bindable var model: ServerModel
    @AppStorage(SettingsView.refreshIntervalKey) private var refreshInterval = SettingsView
        .defaultRefreshInterval
    @State private var section = Section.overview

    enum Section: String, CaseIterable, Identifiable {
        case overview = "Overview"
        case results = "Results"
        case brokenLinks = "Broken Links"
        case performance = "Performance"

        var id: Self { self }
    }

    var body: some View {
        Group {
            switch section {
            case .overview:
                OverviewView(model: model)
            case .results:
                ResultsView(model: model)
            case .brokenLinks:
                BrokenLinksView(model: model)
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
                VStack(alignment: .leading, spacing: 0) {
                    Text(model.server.name)
                        .font(.headline)
                    Text(model.server.baseURL.absoluteString)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                .lineLimit(1)
                .padding(.horizontal, 6)
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
            ToolbarItem(placement: .principal) {
                Picker("Section", selection: $section) {
                    ForEach(Section.allCases) { section in
                        Text(section.rawValue).tag(section)
                    }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .background {
                    CrawlProgressBar(summary: model.current)
                }
                .help(CrawlProgressBar.help(for: model.current))
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
