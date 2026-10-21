import SwiftUI

/// StatusBar shows the central facts of the overview at the bottom of the
/// window - the current scrape, its status codes and the last complete one -
/// with the crawl progress behind them.
struct StatusBar: View {
    static let height: CGFloat = 26

    let current: WalkerVO.ScrapeSummary?
    let complete: WalkerVO.ScrapeSummary?
    let connection: ServerModel.Connection

    var body: some View {
        HStack(spacing: 16) {
            state
            if let current, !(current.statusCodes ?? [:]).isEmpty {
                Divider()
                    .frame(height: 12)
                StatusClasses(summary: current)
            }
            Spacer(minLength: 8)
            if let complete {
                lastComplete(complete)
            }
        }
        .font(.caption)
        .monospacedDigit()
        .lineLimit(1)
        .padding(.horizontal, 12)
        .frame(height: Self.height)
        .frame(maxWidth: .infinity)
        .background {
            CrawlProgressBar(summary: current, opacity: 0.22)
        }
        .background(.bar)
        .overlay(alignment: .top) {
            Divider()
        }
        .help(CrawlProgressBar.help(for: current))
    }

    @ViewBuilder
    private var state: some View {
        switch connection {
        case let .offline(message):
            Label {
                Text("Offline · \(message)")
                    .truncationMode(.tail)
            } icon: {
                Circle().fill(.red).frame(width: 7, height: 7)
            }
        case .unknown:
            Label {
                Text("Connecting …")
            } icon: {
                Circle().fill(.gray).frame(width: 7, height: 7)
            }
        case .online:
            if let current, current.running {
                Label {
                    HStack(spacing: 6) {
                        Text("Crawling · \(current.pages.formatted()) of \((current.pages + current.jobsOpen).formatted()) pages")
                        if let started = current.started {
                            Text(started, style: .timer)
                                .foregroundStyle(.secondary)
                        }
                        Text("\(current.scrapeSpeedAverage, format: .number.precision(.fractionLength(1))) req/s")
                            .foregroundStyle(.secondary)
                    }
                } icon: {
                    Circle()
                        .fill(.green)
                        .frame(width: 7, height: 7)
                        // pulses while crawling
                        .phaseAnimator([1.0, 0.3]) { dot, opacity in dot.opacity(opacity) } animation: { _ in .easeInOut(duration: 0.9) }
                }
            } else if let next = complete?.nextCycle {
                Label {
                    Text("Idle · next scrape \(next, style: .relative)")
                } icon: {
                    Circle().fill(.blue).frame(width: 7, height: 7)
                }
            } else {
                Label {
                    Text("Idle")
                } icon: {
                    Circle().fill(.blue).frame(width: 7, height: 7)
                }
            }
        }
    }

    private func lastComplete(_ complete: WalkerVO.ScrapeSummary) -> some View {
        HStack(spacing: 6) {
            Text("Last complete")
                .foregroundStyle(.secondary)
            if let completed = complete.completed {
                Text(completed, format: .dateTime.day().month().hour().minute())
            }
            Text("· \(complete.pages.formatted()) pages")
            if let elapsed = complete.elapsed() {
                Text("· \(elapsed.formatted(.units(allowed: [.hours, .minutes, .seconds], width: .narrow, maximumUnitCount: 2)))")
                    .foregroundStyle(.secondary)
            }
        }
    }
}

/// StatusClasses counts the pages of a scrape per status class - 2xx, 3xx, …
struct StatusClasses: View {
    let summary: WalkerVO.ScrapeSummary

    var body: some View {
        HStack(spacing: 10) {
            ForEach(CrawlProgressBar.segments(of: summary)) { segment in
                HStack(spacing: 3) {
                    Circle()
                        .fill(segment.color)
                        .frame(width: 6, height: 6)
                    Text("\(segment.pages.formatted()) \(segment.id)")
                }
            }
        }
    }
}
