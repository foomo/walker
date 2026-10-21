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
