import Charts
import SwiftUI

struct OverviewView: View {
    let model: ServerModel

    @Environment(LocalWalkers.self) private var walkers
    @State private var confirmsRestart = false
    @State private var isRestarting = false

    var body: some View {
        if model.server.isLocal {
            // a local server: its scrapes above the log of its walker, its config beside them
            HSplitView {
                VSplitView {
                    scrapes
                        .frame(minHeight: 200, maxHeight: .infinity)
                    LocalWalkerLogPane(server: model.server)
                        .frame(minHeight: 100, idealHeight: 240, maxHeight: .infinity)
                }
                .frame(minWidth: 360, idealWidth: 520, maxWidth: .infinity)
                LocalConfigForm(server: model.server)
                    .frame(minWidth: 380, idealWidth: 480, maxWidth: .infinity)
            }
        } else {
            scrapes
        }
    }

    private var scrapes: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                // a stopped local walker is not offline - the config says why it is not running
                if case let .offline(message) = model.connection, !model.server.isLocal || walkers.isActive(model.server.id) {
                    Label(message, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.red)
                        .padding(.horizontal)
                }
                restartBar
                ScrapeCard(title: "Current scrape", systemImage: "arrow.trianglehead.2.clockwise", summary: model.current) {
                    if let next = model.complete?.nextCycle {
                        Text("Idle – the next scrape starts \(next, style: .relative)")
                    } else {
                        Text("No scrape is running.")
                    }
                }
                ScrapeCard(title: "Last complete scrape", systemImage: "checkmark.circle", summary: model.complete) {
                    Text("No scrape has completed yet.")
                }
            }
            .padding()
        }
    }
}

extension OverviewView {
    /// restartBar forces a new scrape cycle - a running one is aborted, after a question
    var restartBar: some View {
        HStack {
            if let error = model.restartError {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.red)
            }
            Spacer()
            if isRestarting {
                ProgressView()
                    .controlSize(.small)
            }
            Button("New Scrape", systemImage: "arrow.clockwise") {
                if model.current?.running == true {
                    confirmsRestart = true
                } else {
                    restart()
                }
            }
            .disabled(model.connection != .online || isRestarting)
            .help("Start a new scrape cycle now - a running one is aborted, the last complete scrape stays")
            .confirmationDialog("Abort the running scrape?", isPresented: $confirmsRestart) {
                Button("Abort and Start New Scrape", role: .destructive) {
                    restart()
                }
            } message: {
                Text("\(model.current?.pages.formatted() ?? "0") pages of the running scrape are dropped. The last complete scrape stays.")
            }
        }
        .padding(.horizontal)
    }

    private func restart() {
        isRestarting = true
        Task {
            await model.restartScrape()
            isRestarting = false
        }
    }
}

/// ScrapeCard shows the same facts for the current and the last complete scrape.
struct ScrapeCard<Empty: View>: View {
    let title: String
    let systemImage: String
    let summary: WalkerVO.ScrapeSummary?
    @ViewBuilder var empty: Empty

    var body: some View {
        GroupBox {
            Group {
                if let summary {
                    ScrapeSummaryView(summary: summary)
                } else {
                    empty
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .padding(8)
        } label: {
            Label(title, systemImage: systemImage)
                .font(.headline)
        }
    }
}

struct ScrapeSummaryView: View {
    let summary: WalkerVO.ScrapeSummary

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if summary.running {
                ProgressView(value: Double(summary.pages), total: Double(max(summary.pages + summary.jobsOpen, 1))) {
                    Text("\(summary.pages) pages scraped, \(summary.jobsOpen) open")
                }
            }
            Grid(alignment: .leading, horizontalSpacing: 24, verticalSpacing: 6) {
                GridRow {
                    fact("Pages", Text(summary.pages, format: .number))
                    fact("Open", Text(summary.jobsOpen, format: .number))
                    fact("Speed", Text("\(summary.scrapeSpeedAverage, format: .number.precision(.fractionLength(1))) req/s"))
                }
                GridRow {
                    if let started = summary.started {
                        fact("Started", Text(started, format: .dateTime))
                    }
                    if let completed = summary.completed {
                        fact("Completed", Text(completed, format: .dateTime))
                    }
                    if summary.running, let started = summary.started {
                        fact("Running for", Text(started, style: .timer))
                    } else if let elapsed = summary.elapsed() {
                        fact("Took", Text(elapsed.formatted(.time(pattern: .hourMinuteSecond))))
                    }
                }
                if summary.cycleDeadline != nil || summary.nextCycle != nil {
                    GridRow {
                        if let deadline = summary.cycleDeadline {
                            fact("Deadline", Text(deadline, format: .dateTime))
                        }
                        if !summary.running, let next = summary.nextCycle {
                            fact("Next scrape", Text(next, format: .dateTime))
                        }
                    }
                }
            }
            if !summary.sortedStatusCodes.isEmpty {
                StatusCodeChart(statusCodes: summary.sortedStatusCodes)
            }
            if !(summary.performance ?? []).isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    HStack {
                        Text("Performance")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        Spacer()
                        if let within = summary.share(within: GoTime.Duration(rawValue: 200_000_000)) {
                            Text("\(within, format: .percent.precision(.fractionLength(0))) within 200 ms")
                                .font(.caption.monospacedDigit())
                                .foregroundStyle(.secondary)
                        }
                    }
                    PerformanceStrip(shares: summary.mergedBuckets.shares())
                }
            }
        }
    }

    private func fact(_ label: String, _ value: Text) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label)
                .font(.caption)
                .foregroundStyle(.secondary)
            value
                .monospacedDigit()
        }
    }
}

struct StatusCodeChart: View {
    let statusCodes: [(code: Int, count: Int)]

    var body: some View {
        Chart(statusCodes, id: \.code) { entry in
            BarMark(
                x: .value("Pages", entry.count),
                y: .value("Status", StatusClass.label(for: entry.code))
            )
            .foregroundStyle(StatusClass.color(for: entry.code))
            .annotation(position: .trailing) {
                Text(entry.count, format: .number)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(height: CGFloat(statusCodes.count) * 28 + 20)
    }
}
