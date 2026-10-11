import Foundation
import Observation

/// ServerStore keeps the list of walker servers, persisted in the user defaults.
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
