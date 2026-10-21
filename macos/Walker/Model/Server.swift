import Foundation

/// Server is a walker instance the app connects to.
struct Server: Identifiable, Codable, Hashable, Sendable {
    var id = UUID()
    var name: String
    var baseURL: URL
    /// configPath is the yaml of a local walker, started by the app with the
    /// bundled walker - nil for a remote server. Its baseURL follows the addr of the config.
    var configPath: String?

    var isLocal: Bool { configPath != nil }

    var configURL: URL? { configPath.map { URL(filePath: $0) } }

    static let local = Server(name: "Local walker", baseURL: URL(string: "http://localhost:3001")!)

    /// localURL is where a walker listening on addr is reached: ":3001" and
    /// "0.0.0.0:3001" are localhost, "127.0.0.1:3001" keeps its host
    static func localURL(addr: String) -> URL? {
        guard let colon = addr.lastIndex(of: ":") else { return nil }
        let port = addr[addr.index(after: colon)...]
        guard let number = Int(port), number > 0 else { return nil }
        var host = String(addr[..<colon])
        if ["", "0.0.0.0", "[::]", "::"].contains(host) {
            host = "localhost"
        }
        return URL(string: "http://\(host):\(number)")
    }
}
