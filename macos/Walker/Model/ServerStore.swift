import Foundation
import Observation

/// ServerStore keeps the list of walker servers, persisted in the user defaults -
/// remote servers and the local configs the app runs walkers for.
@MainActor
@Observable
final class ServerStore {
    private static let defaultsKey = "servers"

    var servers: [Server] {
        didSet { save() }
    }

    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private var models: [Server.ID: ServerModel] = [:]

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        if let data = defaults.data(forKey: Self.defaultsKey),
           let servers = try? JSONDecoder().decode([Server].self, from: data) {
            self.servers = servers
        } else {
            servers = [.local]
        }
    }

    var localServers: [Server] { servers.filter(\.isLocal) }

    var remoteServers: [Server] { servers.filter { !$0.isLocal } }

    func server(_ id: Server.ID?) -> Server? {
        servers.first { $0.id == id }
    }

    /// model returns the one model per server, so that its state survives switching the selection
    func model(for server: Server) -> ServerModel {
        if let model = models[server.id], model.server == server {
            return model
        }
        let model = ServerModel(server: server)
        models[server.id] = model
        return model
    }

    func add(_ server: Server) {
        servers.append(server)
    }

    /// localServer is the local server of a config file, if it is tracked
    func localServer(configURL: URL) -> Server? {
        let path = configURL.standardizedFileURL.path(percentEncoded: false)
        return servers.first { $0.configPath == path }
    }

    /// addLocal tracks a config file - once per path, a known one is returned as it is
    @discardableResult
    func addLocal(configURL: URL) -> Server {
        if let known = localServer(configURL: configURL) {
            return known
        }
        let server = Server(
            name: configURL.deletingPathExtension().lastPathComponent,
            baseURL: Server.localURL(addr: ConfigTool.Form().addr)!,
            configPath: configURL.standardizedFileURL.path(percentEncoded: false)
        )
        servers.append(server)
        return server
    }

    /// setShapeRoot writes the shape root into the config of a local server -
    /// always relative to the config file, walker resolves it against it
    func setShapeRoot(_ id: Server.ID, path: String) async throws {
        try await updateConfig(id) { form, base in
            form.shapeRoot = ConfigTool.relativePath(path, from: base)
        }
    }

    /// setGroupHeader writes the group header into the config of a local server
    func setGroupHeader(_ id: Server.ID, header: String) async throws {
        try await updateConfig(id) { form, _ in
            form.groupHeader = header
        }
    }

    /// updateConfig changes the form of the config of a local server and
    /// writes it - comments and unknown keys of the config stay. base is the
    /// directory of the config.
    private func updateConfig(_ id: Server.ID, _ change: (inout ConfigTool.Form, _ base: String) -> Void) async throws {
        guard let url = server(id)?.configURL else {
            throw CocoaError(.fileNoSuchFile)
        }
        let yaml = try String(contentsOf: url, encoding: .utf8)
        let base = url.deletingLastPathComponent().path(percentEncoded: false)
        var form = try await ConfigTool.read(yaml: yaml, base: base).form
        change(&form, base)
        let written = try await ConfigTool.write(yaml: yaml, form: form)
        try written.write(to: url, atomically: true, encoding: .utf8)
    }

    /// refreshLocal reads and validates a local config - the address of the
    /// server follows its addr
    @discardableResult
    func refreshLocal(_ id: Server.ID) async throws -> ConfigTool.ReadResult {
        guard let url = server(id)?.configURL else {
            throw CocoaError(.fileNoSuchFile)
        }
        let yaml = try String(contentsOf: url, encoding: .utf8)
        let result = try await ConfigTool.read(yaml: yaml, base: url.deletingLastPathComponent().path(percentEncoded: false))
        if let baseURL = Server.localURL(addr: result.form.addr),
           let index = servers.firstIndex(where: { $0.id == id }),
           servers[index].baseURL != baseURL {
            servers[index].baseURL = baseURL
        }
        return result
    }

    /// sampleMetrics samples the /metrics of all servers in the background, every
    /// refresh interval, until cancelled - all at once, a slow one does not
    /// hold up the others. Local servers only while their walker runs.
    func sampleMetrics(walkers: LocalWalkers) async {
        while !Task.isCancelled {
            let models = servers
                .filter { !$0.isLocal || walkers.state($0.id) == .running }
                .map { model(for: $0) }
            await withTaskGroup(of: Void.self) { group in
                for model in models {
                    group.addTask { await model.liveMetrics.sample() }
                }
            }
            let interval = UserDefaults.standard.object(forKey: SettingsView.refreshIntervalKey) as? Double ?? SettingsView.defaultRefreshInterval
            try? await Task.sleep(for: .seconds(max(interval, 1)))
        }
    }

    func remove(_ id: Server.ID) {
        servers.removeAll { $0.id == id }
        models[id] = nil
    }

    private func save() {
        if let data = try? JSONEncoder().encode(servers) {
            defaults.set(data, forKey: Self.defaultsKey)
        }
    }
}
