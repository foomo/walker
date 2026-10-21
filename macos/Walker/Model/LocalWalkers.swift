import AppKit
import Foundation
import Observation

/// LocalWalkers runs the walkers of the local configs - one process of the
/// walker helper bundled in Contents/Helpers per config. A walker keeps the
/// read end of a pipe as its stdin and exits when the app's end closes, so
/// none outlives the app, not even a crashed one.
@MainActor
@Observable
final class LocalWalkers {
    enum State: Equatable {
        case stopped
        case starting
        case running
        case stopping
        case failed(String)
    }

    /// LogLine is a line walker wrote, with the time it arrived
    struct LogLine: Identifiable, Hashable, Sendable {
        let id: Int
        let date: Date
        let text: String
    }

    /// logLinesKey is the setting of how many lines a log keeps - older ones are dropped
    static let logLinesKey = "localWalkerLogLines"
    static let defaultLogLines = 10_000

    static var logLines: Int {
        let value = UserDefaults.standard.integer(forKey: logLinesKey)
        return value > 0 ? max(value, 100) : defaultLogLines
    }

    private(set) var states: [Server.ID: State] = [:]
    /// logs are the last lines walker wrote to stdout and stderr - kept after it exits
    private(set) var logs: [Server.ID: [LogLine]] = [:]

    @ObservationIgnored private var processes: [Server.ID: Process] = [:]
    /// stdins hold the app's end of each walker's stdin
    @ObservationIgnored private var stdins: [Server.ID: Pipe] = [:]
    /// partialLines is output after the last newline, completed by the next chunk
    @ObservationIgnored private var partialLines: [Server.ID: String] = [:]
    /// lineCount numbers the log lines - ids stay unique across runs and clears
    @ObservationIgnored private var lineCount = 0

    static var helperURL: URL {
        Bundle.main.bundleURL.appending(path: "Contents/Helpers/walker")
    }

    func state(_ id: Server.ID) -> State {
        states[id] ?? .stopped
    }

    func log(_ id: Server.ID) -> [LogLine] {
        logs[id] ?? []
    }

    func clearLog(_ id: Server.ID) {
        logs[id] = []
    }

    /// isActive is true from start to exit
    func isActive(_ id: Server.ID) -> Bool {
        switch state(id) {
        case .starting, .running, .stopping: true
        case .stopped, .failed: false
        }
    }

    /// start validates the config and launches walker in the config's
    /// directory - a relative schema root resolves like in the config editor.
    /// It is running once its service answers.
    func start(_ id: Server.ID, store: ServerStore) async {
        guard !isActive(id) else { return }
        states[id] = .starting
        logs[id] = []
        partialLines[id] = nil

        let helper = Self.helperURL
        guard FileManager.default.isExecutableFile(atPath: helper.path(percentEncoded: false)) else {
            return fail(id, "The walker helper is missing - build the app with Go installed.")
        }
        guard let configURL = store.server(id)?.configURL,
              FileManager.default.fileExists(atPath: configURL.path(percentEncoded: false))
        else {
            return fail(id, "The config file was not found.")
        }
        do {
            let result = try await store.refreshLocal(id)
            if let error = result.problems.first(where: { $0.severity == "error" }) {
                return fail(id, "The config has errors: \(error.message) (line \(error.line))")
            }
        } catch {
            return fail(id, error.localizedDescription)
        }
        guard let server = store.server(id) else { return fail(id, "The config was removed.") }

        // walker would fail on a taken port - but the one listening there
        // would answer, and look like this walker
        let model = store.model(for: server)
        await model.refreshScrapes()
        if model.connection == .online {
            return fail(id, "Another walker already listens on \(server.baseURL.absoluteString) - stop it or change the addr of the config.")
        }
        guard state(id) == .starting else { return }

        let process = Process()
        process.executableURL = helper
        process.arguments = ["-exit-on-stdin-close", configURL.path(percentEncoded: false)]
        process.currentDirectoryURL = configURL.deletingLastPathComponent()
        let stdin = Pipe(), output = Pipe()
        process.standardInput = stdin
        process.standardOutput = output
        process.standardError = output
        output.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            if data.isEmpty {
                handle.readabilityHandler = nil
                return
            }
            // the main queue keeps the chunks in order
            let walkers = self
            DispatchQueue.main.async {
                MainActor.assumeIsolated { walkers?.append(id, data) }
            }
        }
        process.terminationHandler = { [weak self] process in
            let status = process.terminationStatus
            let walkers = self
            DispatchQueue.main.async {
                MainActor.assumeIsolated { walkers?.exited(id, status: status) }
            }
        }
        do {
            try process.run()
        } catch {
            return fail(id, error.localizedDescription)
        }
        processes[id] = process
        stdins[id] = stdin

        // walker loads the config, then listens - up to 15 s, a schema root takes a moment
        for _ in 0..<50 {
            try? await Task.sleep(for: .milliseconds(300))
            guard state(id) == .starting else { return }
            await model.refreshScrapes()
            if model.connection == .online, state(id) == .starting {
                states[id] = .running
                return
            }
        }
        if state(id) == .starting {
            // alive, but silent - the log tells why
            states[id] = .running
        }
    }

    /// stop asks walker to exit, after 3 s it is killed
    func stop(_ id: Server.ID) {
        guard let process = processes[id] else {
            if state(id) == .starting {
                states[id] = .stopped
            }
            return
        }
        states[id] = .stopping
        process.terminate()
        Task {
            try? await Task.sleep(for: .seconds(3))
            if processes[id] === process, process.isRunning {
                kill(process.processIdentifier, SIGKILL)
            }
        }
    }

    /// stopAll ends all walkers when the app quits - closing their stdin would too
    func stopAll() {
        for (id, process) in processes {
            states[id] = .stopping
            process.terminate()
        }
    }

    private func fail(_ id: Server.ID, _ message: String) {
        states[id] = .failed(message)
    }

    private func exited(_ id: Server.ID, status: Int32) {
        processes[id] = nil
        stdins[id] = nil
        if let rest = partialLines.removeValue(forKey: id), !rest.isEmpty {
            appendLines(id, [rest])
        }
        switch state(id) {
        case .stopping, .stopped:
            states[id] = .stopped
        default:
            let last = log(id).last(where: { !$0.text.trimmingCharacters(in: .whitespaces).isEmpty })?.text
            states[id] = .failed("walker exited with status \(status)" + (last.map { ": \($0)" } ?? ""))
        }
    }

    private func append(_ id: Server.ID, _ data: Data) {
        var lines = ((partialLines[id] ?? "") + String(decoding: data, as: UTF8.self))
            .split(separator: "\n", omittingEmptySubsequences: false)
            .map(String.init)
        partialLines[id] = lines.removeLast()
        appendLines(id, lines)
    }

    private func appendLines(_ id: Server.ID, _ lines: [String]) {
        var log = logs[id] ?? []
        let now = Date.now
        for line in lines {
            lineCount += 1
            log.append(LogLine(id: lineCount, date: now, text: line))
        }
        let limit = Self.logLines
        if log.count > limit {
            log.removeFirst(log.count - limit)
        }
        logs[id] = log
    }
}

/// AppDelegate owns what lives as long as the app: the servers, the local
/// walkers - stopped when the app quits - and the sampling of the live metrics
@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let store = ServerStore()
    let walkers = LocalWalkers()
    private var sampling: Task<Void, Never>?

    func applicationDidFinishLaunching(_ notification: Notification) {
        sampling = Task { [store, walkers] in
            await store.sampleMetrics(walkers: walkers)
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        sampling?.cancel()
        walkers.stopAll()
    }
}
