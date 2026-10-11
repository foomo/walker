import SwiftUI

struct PerformanceView: View {
    let model: ServerModel
    private var summary: ScrapeSummary? {
        switch model.scrape {
        case .current: model.current
        case .complete: model.complete
        }
    }

    var body: some View {
        Group {
            if let summary, !(summary.performance ?? []).isEmpty {
                ScrollView {
                    PerformanceDetails(summary: summary)
                        .padding()
                }
            } else {
                ContentUnavailableView(
                    model.scrape == .current ? "No Scrape Running" : "No Complete Scrape",
                    systemImage: "gauge.with.dots.needle.33percent",
                    description: Text(model.scrape == .current ? "Switch to the last complete scrape, or wait for the next one." : "The first scrape has not completed yet.")
                )
            }
        }
    }
}

/// PerformanceDetails renders the performance of one scrape - the same for current and complete.
struct PerformanceDetails: View {
    let summary: ScrapeSummary

    private var groups: [GroupPerformance] {
        // slowest groups first - that is where to look
        (summary.performance ?? []).sorted { $0.p90.rawValue > $1.p90.rawValue }
    }

    private var shares: [BucketShare] { summary.mergedBuckets.shares() }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            GroupBox {
                VStack(alignment: .leading, spacing: 12) {
                    HStack(alignment: .firstTextBaseline) {
                        if let within = summary.share(within: Duration(rawValue: 200_000_000)) {
                            Text(within, format: .percent.precision(.fractionLength(0)))
                                .font(.system(.largeTitle, design: .rounded).weight(.semibold))
                                .foregroundStyle(within >= 0.9 ? .green : within >= 0.6 ? .orange : .red)
                                .contentTransition(.numericText())
                            Text("of \(summary.pages) pages within 200 ms")
                                .foregroundStyle(.secondary)
                        }
                        Spacer()
                    }
                    PerformanceHistogram(shares: shares)
                        .frame(height: 220)
                    PerformanceLegend(shares: shares)
                }
                .padding(8)
            } label: {
                Label("All pages", systemImage: "chart.bar.xaxis")
                    .font(.headline)
            }

            GroupBox {
                VStack(spacing: 0) {
                    ForEach(groups, id: \.group) { group in
                        GroupPerformanceRow(performance: group)
                        if group.group != groups.last?.group {
                            Divider()
                        }
                    }
                }
                .padding(8)
            } label: {
                Label("By group, slowest first", systemImage: "square.stack.3d.up")
                    .font(.headline)
            }
        }
    }
}
