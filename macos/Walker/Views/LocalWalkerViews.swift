import AppKit
import SwiftUI

/// LocalWalkerButton starts and stops the walker of a local config
struct LocalWalkerButton: View {
    let server: Server

    @Environment(ServerStore.self) private var store
    @Environment(LocalWalkers.self) private var walkers

    var body: some View {
        if walkers.isActive(server.id) {
            Button("Stop", systemImage: "stop.fill") {
                walkers.stop(server.id)
            }
            .disabled(walkers.state(server.id) == .stopping)
            .help("Stop the walker of \(server.name)")
        } else {
            Button("Start", systemImage: "play.fill") {
                Task { await walkers.start(server.id, store: store) }
            }
            .help("Start a walker with the saved config \(server.name)")
        }
    }
}

/// LocalWalkerLog shows walker's output from the top, a line per row with
/// the time it arrived - it follows new lines at the bottom
struct LocalWalkerLog: View {
    let lines: [LocalWalkers.LogLine]

    var body: some View {
        ScrollView([.vertical, .horizontal]) {
            LazyVStack(alignment: .leading, spacing: 2) {
                ForEach(lines) { line in
                    LogLineRow(line: line)
                }
            }
            .padding(10)
        }
        .defaultScrollAnchor(.topLeading, for: .alignment)
        .defaultScrollAnchor(.bottomLeading, for: .initialOffset)
        .defaultScrollAnchor(.bottomLeading, for: .sizeChanges)
    }
}

private struct LogLineRow: View {
    let line: LocalWalkers.LogLine

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Text(line.date, format: .dateTime.hour(.twoDigits(amPM: .omitted)).minute(.twoDigits).second(.twoDigits))
                .foregroundStyle(.tertiary)
                .fixedSize()
            if let entry = SlogEntry(line.text) {
                Text(entry.level)
                    .foregroundStyle(Self.color(level: entry.level))
                    .fixedSize()
                    .frame(width: 40, alignment: .leading)
                Text(Self.styled(entry))
                    .fixedSize()
            } else {
                Text(line.text.isEmpty ? " " : line.text)
                    .foregroundStyle(style)
                    .fixedSize()
            }
        }
        // a row is never squeezed - a line wider than the log scrolls instead
        // of wrapping the time column into a tall stack of digits
        .lineLimit(1)
        .fixedSize(horizontal: true, vertical: false)
        .font(.callout.monospaced())
        .textSelection(.enabled)
    }

    /// styled is the message, then the attributes - keys faint, values dimmed
    private static func styled(_ entry: SlogEntry) -> AttributedString {
        var text = AttributedString(entry.message)
        for attribute in entry.attributes {
            var key = AttributedString("  \(attribute.key)=")
            key.foregroundColor = Color(nsColor: .tertiaryLabelColor)
            var value = AttributedString(attribute.value)
            value.foregroundColor = Color(nsColor: .secondaryLabelColor)
            text += key
            text += value
        }
        return text
    }

    private static func color(level: String) -> Color {
        switch level {
        case "ERROR": .red
        case "WARN": .orange
        case "DEBUG": .secondary
        default: .blue
        }
    }

    /// style marks problems in lines that are not slog entries - a panic, a stack trace
    private var style: AnyShapeStyle {
        let text = line.text.lowercased()
        if text.contains("error") || text.contains("panic") || text.contains("fatal") || text.contains("failed") {
            return AnyShapeStyle(.red)
        }
        if text.contains("warn") {
            return AnyShapeStyle(.orange)
        }
        return AnyShapeStyle(.primary)
    }
}

/// SlogEntry is a line of walker's slog text handler:
/// time=… level=INFO msg="scrape cycle completed" pages=23 duration=43.7ms -
/// the time is dropped, the log view has the time a line arrived
struct SlogEntry: Equatable {
    var level = ""
    var message = ""
    var attributes: [(key: String, value: String)] = []

    init?(_ line: String) {
        guard line.hasPrefix("time=") || line.hasPrefix("level=") else { return nil }
        var rest = Substring(line)
        while let pair = Self.pair(&rest) {
            switch pair.key {
            case "time": continue
            case "level": level = pair.value
            case "msg": message = pair.value
            default: attributes.append(pair)
            }
        }
        guard !level.isEmpty, rest.isEmpty else { return nil }
    }

    static func == (lhs: SlogEntry, rhs: SlogEntry) -> Bool {
        lhs.level == rhs.level && lhs.message == rhs.message
            && lhs.attributes.map(\.key) == rhs.attributes.map(\.key)
            && lhs.attributes.map(\.value) == rhs.attributes.map(\.value)
    }

    /// pair reads the next key=value - a value is bare or a quoted Go string
    private static func pair(_ rest: inout Substring) -> (key: String, value: String)? {
        rest = rest.drop(while: { $0 == " " })
        guard let equals = rest.firstIndex(of: "="), !rest[..<equals].contains(" ") else { return nil }
        let key = String(rest[..<equals])
        rest = rest[rest.index(after: equals)...]
        guard rest.first == "\"" else {
            let end = rest.firstIndex(of: " ") ?? rest.endIndex
            defer { rest = rest[end...] }
            return (key, String(rest[..<end]))
        }
        var value = ""
        var index = rest.index(after: rest.startIndex)
        while index < rest.endIndex {
            let character = rest[index]
            if character == "\"" {
                rest = rest[rest.index(after: index)...]
                return (key, value)
            }
            if character == "\\", rest.index(after: index) < rest.endIndex {
                index = rest.index(after: index)
                switch rest[index] {
                case "n": value.append("\n")
                case "t": value.append("\t")
                default: value.append(rest[index])
                }
            } else {
                value.append(character)
            }
            index = rest.index(after: index)
        }
        return nil
    }
}

/// LocalConfigForm is the config of a local server, edited in place in its
/// overview - with the state of its walker on top. Edits are saved to the
/// file right away, the walker reads it on its next start.
struct LocalConfigForm: View {
    let server: Server

    @Environment(ServerStore.self) private var store
    @Environment(LocalWalkers.self) private var walkers
    /// text is the config being edited, nil until the file is read
    @State private var text: String?
    /// disk is the text and modification date of the file as last read or written
    @State private var disk: (text: String, modified: Date?)?
    @State private var problems: [ConfigTool.Problem] = []
    @State private var error: String?

    private var exists: Bool {
        guard let url = server.configURL else { return false }
        return FileManager.default.fileExists(atPath: url.path(percentEncoded: false))
    }

    var body: some View {
        Group {
            if !exists {
                ContentUnavailableView {
                    Label("Config Not Found", systemImage: "doc.questionmark")
                } description: {
                    Text(server.configPath ?? "")
                } actions: {
                    Button("Remove", role: .destructive) {
                        store.remove(server.id)
                    }
                }
            } else if let url = server.configURL, text != nil {
                ConfigFormView(
                    text: Binding(get: { text ?? "" }, set: { text = $0 }),
                    base: url.deletingLastPathComponent().path(percentEncoded: false),
                    problems: $problems
                ) {
                    status
                } footer: {
                    EmptyView()
                }
            } else {
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .onChange(of: text) {
            save()
        }
        // edits from elsewhere - another editor - are read in; while this
        // one types, its own saves are the newest
        .task(id: server.configPath) {
            while !Task.isCancelled {
                load()
                try? await Task.sleep(for: .seconds(1))
            }
        }
    }

    @ViewBuilder
    private var status: some View {
        Section {
            switch walkers.state(server.id) {
            case let .failed(message):
                Label(message, systemImage: "xmark.octagon")
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
            case .starting:
                Label("Starting…", systemImage: "hourglass")
            case .stopping:
                Label("Stopping…", systemImage: "hourglass")
            case .running:
                Label("Running - changes apply with the next start", systemImage: "play.circle")
                    .foregroundStyle(.secondary)
            case .stopped:
                Label("Not running - changes apply with the next start", systemImage: "pause.circle")
                    .foregroundStyle(.secondary)
            }
            if let error {
                Label(error, systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.orange)
            }
            LabeledContent("File") {
                HStack {
                    Text(server.configPath ?? "")
                        .lineLimit(1)
                        .truncationMode(.middle)
                        .textSelection(.enabled)
                    Button("Show in Finder", systemImage: "folder") {
                        showInFinder(server)
                    }
                    .labelStyle(.iconOnly)
                    .buttonStyle(.borderless)
                    .help("Show in Finder")
                }
            }
            LabeledContent("Address", value: server.baseURL.absoluteString)
            LabeledContent("Config", value: ConfigTool.Problem.summary(problems))
        }
    }

    private func modified(_ url: URL) -> Date? {
        (try? FileManager.default.attributesOfItem(atPath: url.path(percentEncoded: false)))?[.modificationDate] as? Date
    }

    /// load reads the file when it changed on disk
    private func load() {
        guard let url = server.configURL, exists else { return }
        let date = modified(url)
        if let disk, disk.modified == date { return }
        do {
            let content = try String(contentsOf: url, encoding: .utf8)
            disk = (content, date)
            if text != content {
                text = content
            }
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// save writes edits of the form to the file - and the address follows the addr
    private func save() {
        guard let url = server.configURL, let text, text != disk?.text else { return }
        do {
            try text.write(to: url, atomically: true, encoding: .utf8)
            disk = (text, modified(url))
            error = nil
        } catch {
            self.error = error.localizedDescription
            return
        }
        // a running walker keeps its address until the next start
        if !walkers.isActive(server.id) {
            Task { try? await store.refreshLocal(server.id) }
        }
    }
}

/// LocalWalkerLogPane is the output of the running walker, or of its last
/// run - filterable, to copy and to clear
struct LocalWalkerLogPane: View {
    let server: Server

    @Environment(LocalWalkers.self) private var walkers
    @State private var filter = ""

    private var lines: [LocalWalkers.LogLine] {
        let all = walkers.log(server.id)
        let filter = filter.trimmingCharacters(in: .whitespaces)
        return filter.isEmpty ? all : all.filter { $0.text.localizedCaseInsensitiveContains(filter) }
    }

    var body: some View {
        let all = walkers.log(server.id)
        let lines = lines
        VStack(spacing: 0) {
            HStack(spacing: 10) {
                Text(walkers.isActive(server.id) ? "Log" : "Log of the last run")
                    .font(.headline)
                Spacer()
                Text(lines.count == all.count ? "\(all.count) lines" : "\(lines.count) of \(all.count) lines")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
                TextField("Filter", text: $filter, prompt: Text("Filter"))
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 160)
                Button("Copy", systemImage: "doc.on.doc") {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(lines.map(\.text).joined(separator: "\n"), forType: .string)
                }
                .help("Copy the shown lines")
                Button("Clear", systemImage: "trash") {
                    walkers.clearLog(server.id)
                }
                .help("Clear the log")
            }
            .labelStyle(.iconOnly)
            .buttonStyle(.borderless)
            .disabled(all.isEmpty)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            Divider()
            if all.isEmpty {
                ContentUnavailableView(
                    "No Output",
                    systemImage: "text.alignleft",
                    description: Text(walkers.isActive(server.id) ? "Walker has not written anything yet." : "Start the walker - what it writes shows up here.")
                )
            } else if lines.isEmpty {
                ContentUnavailableView.search(text: filter)
            } else {
                LocalWalkerLog(lines: lines)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(.background)
    }
}

@MainActor
func showInFinder(_ server: Server) {
    guard let url = server.configURL else { return }
    NSWorkspace.shared.activateFileViewerSelecting([url])
}
