import Foundation

/// Server is a walker instance the app connects to.
struct Server: Identifiable, Codable, Hashable, Sendable {
    var id = UUID()
    var name: String
    var baseURL: URL

    static let local = Server(name: "Local walker", baseURL: URL(string: "http://localhost:3001")!)
}
