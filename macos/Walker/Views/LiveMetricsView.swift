import Charts
import SwiftUI

/// LiveMetricsView charts the prometheus metrics the app samples from the
/// walker in the background - in memory only, the last 30 minutes
struct LiveMetricsView: View {
    let metrics: LiveMetrics

    @AppStorage("liveMetricsWindow") private var window: TimeInterval = 5 * 60

    private static let windows: [(title: String, seconds: TimeInterval)] = [
        ("1 min", 60), ("5 min", 5 * 60), ("15 min", 15 * 60), ("30 min", 30 * 60),
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .firstTextBaseline) {
                Label("Live metrics", systemImage: "waveform.path.ecg")
                    .font(.headline)
                Text(caption)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Picker("Window", selection: $window) {
                    ForEach(Self.windows, id: \.seconds) { window in
                        Text(window.title).tag(window.seconds)
                    }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .fixedSize()
                Button("Clear", systemImage: "trash") {
                    metrics.clear()
                }
                .labelStyle(.iconOnly)
                .buttonStyle(.borderless)
                .help("Forget the samples")
            }
            if let error = metrics.error {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.red)
                    .font(.callout)
            }
            // a minute of the chart redraws once per second, so the window moves on between samples
            TimelineView(.periodic(from: .now, by: 1)) { context in
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 340), spacing: 12)], spacing: 12) {
                    ForEach(MetricsChart.allCases) { chart in
                        MetricsChartCard(
                            chart: chart,
                            points: metrics.points(chart, window: window),
                            domain: context.date.addingTimeInterval(-window)...context.date
                        )
                    }
                }
            }
        }
    }

    private var caption: String {
        guard let first = metrics.snapshots.first else { return "sampled from /metrics in the background, while the walker runs" }
        return "\(metrics.snapshots.count) samples since \(first.date.formatted(date: .omitted, time: .shortened)) · kept in memory for 30 min"
    }
}

/// MetricsChartCard is one chart: lines over time, one axis, a crosshair with the values
private struct MetricsChartCard: View {
    let chart: MetricsChart
    let points: [MetricPoint]
    let domain: ClosedRange<Date>

    @State private var hovered: Date?

    private var series: [String] {
        chart.series.filter { name in points.contains { $0.series == name } }
    }

    var body: some View {
        GroupBox {
            Chart {
                ForEach(points) { point in
                    LineMark(
                        x: .value("Time", point.date),
                        y: .value(chart.title, point.value),
                        series: .value("Series", point.series)
                    )
                    .foregroundStyle(by: .value("Series", point.series))
                    .lineStyle(StrokeStyle(lineWidth: 2, lineCap: .round, lineJoin: .round))
                    .interpolationMethod(.monotone)
                }
                if let hovered, let values = values(near: hovered) {
                    RuleMark(x: .value("Time", values.date))
                        .foregroundStyle(.secondary.opacity(0.5))
                        .lineStyle(StrokeStyle(lineWidth: 1))
                        .annotation(position: .top, spacing: 0, overflowResolution: .init(x: .fit(to: .chart), y: .disabled)) {
                            tooltip(values)
                        }
                }
            }
            .chartForegroundStyleScale(domain: chart.series, range: chart.colors)
            .chartLegend(series.count > 1 ? .visible : .hidden)
            .chartXScale(domain: domain)
            .chartXAxis {
                AxisMarks(values: .automatic(desiredCount: 4)) { _ in
                    AxisGridLine().foregroundStyle(.quaternary)
                    AxisValueLabel(format: .dateTime.hour().minute())
                }
            }
            .chartYAxis {
                AxisMarks(position: .leading, values: .automatic(desiredCount: 4)) { value in
                    AxisGridLine().foregroundStyle(.quaternary)
                    AxisValueLabel {
                        if let number = value.as(Double.self) {
                            Text(chart.format(number))
                        }
                    }
                }
            }
            .chartXSelection(value: $hovered)
            .frame(height: 160)
            .overlay {
                if points.isEmpty {
                    Text(chart.empty)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
            }
        } label: {
            Text(chart.title)
                .font(.subheadline.weight(.semibold))
                .help(chart.promQL)
        }
    }

    /// values are the points of the sample nearest to the hovered time
    private func values(near date: Date) -> (date: Date, points: [MetricPoint])? {
        guard let nearest = points.min(by: { abs($0.date.timeIntervalSince(date)) < abs($1.date.timeIntervalSince(date)) }) else { return nil }
        let atSample = points.filter { $0.date == nearest.date }
        let ordered = chart.series.compactMap { name in atSample.first { $0.series == name } }
        return (nearest.date, ordered)
    }

    private func tooltip(_ values: (date: Date, points: [MetricPoint])) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(values.date, format: .dateTime.hour().minute().second())
                .foregroundStyle(.secondary)
            ForEach(values.points) { point in
                HStack(spacing: 6) {
                    Circle()
                        .fill(chart.color(point.series))
                        .frame(width: 7, height: 7)
                    Text(point.series)
                        .foregroundStyle(.secondary)
                    Spacer(minLength: 8)
                    Text(chart.format(point.value))
                        .monospacedDigit()
                }
            }
        }
        .font(.caption)
        .padding(6)
        .frame(minWidth: 120)
        .background(.regularMaterial, in: .rect(cornerRadius: 6))
    }
}

extension MetricsChart {
    /// series in a fixed order, so that a color always means the same
    var series: [String] {
        switch self {
        case .requestsByStatus: ["2xx", "3xx", "4xx", "5xx", "failed"]
        case .requestDuration: ["p50", "p90", "p99"]
        case .crawlProgress: ["scraped", "open"]
        case .cpu: ["cpu"]
        case .memory: ["from OS", "heap in use", "resident"]
        case .goroutines: ["goroutines"]
        }
    }

    /// colors by the job of the series: status classes keep the colors of the
    /// app, quantiles are one hue from light to dark
    var colors: [Color] {
        switch self {
        case .requestsByStatus:
            [200, 300, 400, 500, 0].map(StatusClass.color(for:))
        case .requestDuration:
            [Color.indigo.opacity(0.4), Color.indigo.opacity(0.7), Color.indigo]
        case .crawlProgress:
            [Color.green, Color.secondary]
        case .memory:
            [Color.accentColor, Color.teal, Color.secondary]
        case .cpu, .goroutines:
            [Color.accentColor]
        }
    }

    func color(_ name: String) -> Color {
        series.firstIndex(of: name).map { colors[$0] } ?? .secondary
    }

    var empty: String {
        switch self {
        case .requestsByStatus, .requestDuration: "No requests in this window"
        default: "No samples yet"
        }
    }

    func format(_ value: Double) -> String {
        switch self {
        case .requestsByStatus:
            "\(value.formatted(.number.precision(.fractionLength(0...1))))/s"
        case .requestDuration:
            value < 1 ? "\(Int((value * 1000).rounded())) ms" : "\(value.formatted(.number.precision(.fractionLength(1)))) s"
        case .crawlProgress, .goroutines:
            value.formatted(.number.notation(.compactName))
        case .cpu:
            value.formatted(.percent.precision(.fractionLength(0)))
        case .memory:
            Measurement(value: value, unit: UnitInformationStorage.bytes)
                .formatted(.byteCount(style: .memory))
        }
    }
}
