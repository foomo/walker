import Charts
import SwiftUI

/// PerformanceStrip is a single 100% bar of the performance buckets.
struct PerformanceStrip: View {
    let shares: [BucketShare]
    var height: CGFloat = 14

    @State private var selectedPages: Int?

    private var total: Int { shares.first?.total ?? 0 }

    /// selected maps the hovered x position (in pages) to its bucket
    private var selected: BucketShare? {
        guard let selectedPages else { return nil }
        var upper = 0
        for share in shares where share.pages > 0 {
            upper += share.pages
            if selectedPages < upper {
                return share
            }
        }
        return shares.last { $0.pages > 0 }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Chart(shares) { share in
                BarMark(
                    x: .value("Pages", share.pages),
                    y: .value("Scrape", "all"),
                    height: .fixed(height)
                )
                .foregroundStyle(share.color)
                .opacity(selected == nil || selected == share ? 1 : 0.35)
            }
            .chartXScale(domain: 0...Swift.max(total, 1))
            .chartXAxis(.hidden)
            .chartYAxis(.hidden)
            .chartLegend(.hidden)
            .chartPlotStyle { plot in
                plot.clipShape(.capsule)
            }
            .chartXSelection(value: $selectedPages)
            .frame(height: height)
            .animation(.smooth, value: shares)

            Group {
                if let selected {
                    Text("\(selected.label) · \(selected.name) · \(selected.pages) pages · \(selected.share, format: .percent.precision(.fractionLength(0)))")
                } else {
                    Text(" ")
                }
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            .lineLimit(1)
        }
    }
}

/// PerformanceHistogram shows one bar per performance bucket.
struct PerformanceHistogram: View {
    let shares: [BucketShare]

    @State private var selectedLabel: String?

    private var selected: BucketShare? {
        shares.first { $0.label == selectedLabel }
    }

    var body: some View {
        Chart {
            ForEach(shares) { share in
                BarMark(
                    x: .value("Duration", share.label),
                    y: .value("Pages", share.pages)
                )
                .foregroundStyle(share.color.gradient)
                .cornerRadius(5)
                .opacity(selected == nil || selected == share ? 1 : 0.4)
                .annotation(position: .top, spacing: 4) {
                    if share.pages > 0 {
                        Text(share.share, format: .percent.precision(.fractionLength(0)))
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            if let selected {
                RuleMark(x: .value("Duration", selected.label))
                    .foregroundStyle(.clear)
                    .annotation(position: .top, spacing: 0, overflowResolution: .init(x: .fit(to: .chart), y: .disabled)) {
                        BucketCallout(share: selected)
                    }
            }
        }
        .chartXSelection(value: $selectedLabel)
        .chartXAxis {
            AxisMarks { _ in
                AxisValueLabel()
                    .font(.caption2)
            }
        }
        .chartYAxis {
            AxisMarks(position: .leading)
        }
        .animation(.smooth, value: shares)
    }
}

struct BucketCallout: View {
    let share: BucketShare

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(share.name)
                .font(.callout.weight(.semibold))
            Text("\(share.label) · \(share.pages) pages · \(share.share, format: .percent.precision(.fractionLength(1)))")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(.regularMaterial, in: .rect(cornerRadius: 8))
        .overlay(alignment: .leading) {
            Capsule()
                .fill(share.color)
                .frame(width: 3)
                .padding(.vertical, 6)
        }
    }
}

/// GroupPerformanceRow shows the distribution and the percentiles of a group.
struct GroupPerformanceRow: View {
    let performance: GroupPerformance

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 16) {
            VStack(alignment: .leading, spacing: 2) {
                Text(performance.group.isEmpty ? "No group" : performance.group)
                    .font(.headline)
                    .lineLimit(1)
                Text("\(performance.pages) pages")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .frame(width: 150, alignment: .leading)

            PerformanceStrip(shares: performance.shares)
                .alignmentGuide(.firstTextBaseline) { $0[VerticalAlignment.center] + 4 }

            HStack(spacing: 14) {
                percentile("P50", performance.p50)
                percentile("P90", performance.p90)
                percentile("Max", performance.max)
            }
        }
        .padding(.vertical, 6)
    }

    private func percentile(_ label: String, _ d: Duration) -> some View {
        VStack(alignment: .trailing, spacing: 2) {
            Text(label)
                .font(.caption2)
                .foregroundStyle(.secondary)
            Text(d.formatted)
                .font(.callout.monospacedDigit().weight(.medium))
                .foregroundStyle(performance.color(of: d))
        }
        .frame(minWidth: 64, alignment: .trailing)
    }
}

/// PerformanceLegend explains the bucket colors.
struct PerformanceLegend: View {
    let shares: [BucketShare]

    var body: some View {
        FlowLayout(spacing: 12) {
            ForEach(shares) { share in
                HStack(spacing: 4) {
                    Circle()
                        .fill(share.color)
                        .frame(width: 8, height: 8)
                    Text(share.label)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                .help(share.name)
            }
        }
    }
}

/// FlowLayout wraps its subviews into rows.
struct FlowLayout: Layout {
    var spacing: CGFloat = 8

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let rows = rows(for: subviews, width: proposal.width ?? .infinity)
        let height = rows.map(\.height).reduce(0, +) + spacing * CGFloat(Swift.max(rows.count - 1, 0))
        let width = rows.map(\.width).max() ?? 0
        return CGSize(width: proposal.width ?? width, height: height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var y = bounds.minY
        for row in rows(for: subviews, width: bounds.width) {
            var x = bounds.minX
            for index in row.indices {
                let size = subviews[index].sizeThatFits(.unspecified)
                subviews[index].place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
                x += size.width + spacing
            }
            y += row.height + spacing
        }
    }

    private struct Row {
        var indices: [Int] = []
        var width: CGFloat = 0
        var height: CGFloat = 0
    }

    private func rows(for subviews: Subviews, width: CGFloat) -> [Row] {
        var rows: [Row] = [Row()]
        for index in subviews.indices {
            let size = subviews[index].sizeThatFits(.unspecified)
            let needed = rows[rows.count - 1].indices.isEmpty ? size.width : rows[rows.count - 1].width + spacing + size.width
            if needed > width, !rows[rows.count - 1].indices.isEmpty {
                rows.append(Row())
            }
            var row = rows[rows.count - 1]
            row.width = row.indices.isEmpty ? size.width : row.width + spacing + size.width
            row.height = Swift.max(row.height, size.height)
            row.indices.append(index)
            rows[rows.count - 1] = row
        }
        return rows
    }
}
