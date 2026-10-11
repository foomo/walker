import Foundation
import GraphLayout
import Observation
import simd

/// GraphModel loads the folder graph of a server and lays it out in 3D with
/// the GraphLayout package - a selected folder expands into its pages.
@MainActor
@Observable
final class GraphModel {
    enum ColorMode: String, CaseIterable, Identifiable {
        case status = "Status"
        case group = "Group"
        case performance = "Performance"

        var id: Self { self }
    }

    /// Selection is a folder of the graph or a page of the expanded folder
    enum Selection: Hashable {
        case folder(Int)
        case page(Int)
    }

    var depth = 2
    var colorMode = ColorMode.status

    private(set) var graph: Graph?
    /// positions of the folders, centered and scaled to a radius of about 1
    private(set) var positions: [SIMD3<Float>] = []
    /// layoutID changes with every new graph - the view rebuilds its scene;
    /// positions move while the layout settles
    private(set) var layoutID = UUID()
    private(set) var isLoading = false
    private(set) var isLayingOut = false
    private(set) var error: String?

    /// the expanded folder - its pages float around it
    private(set) var expanded: Int?
    private(set) var pages: FolderPages?
    /// offsets of the pages from their folder, in the units of positions
    private(set) var pageOffsets: [SIMD3<Float>] = []
    /// expansionID changes with every expanded folder - the view rebuilds its pages
    private(set) var expansionID = UUID()
    private(set) var isExpanding = false

    /// links of the selected page to the folders - see loadPageLinks
    private(set) var pageLinks: [FolderLink] = []
    /// the page of pageLinks, an index into pages
    private(set) var linkedPage: Int?
    /// pageLinksID changes with every change of pageLinks - the view redraws its edges
    private(set) var pageLinksID = UUID()

    @ObservationIgnored private let client: ServiceClient
    /// the query of the current graph - pages are loaded for the same
    @ObservationIgnored private var loaded: Query?

    init(client: ServiceClient) {
        self.client = client
    }

    struct Query: Hashable {
        let scrape: String
        let depth: Int
        let completed: Date?
    }

    func query(scrape: ServerModel.Scrape, completed: Date?) -> Query {
        Query(scrape: scrape.rawValue, depth: depth, completed: completed)
    }

    private func options(_ query: Query) -> GraphOptions {
        GraphOptions(depth: query.depth, globalShare: 0)
    }

    func load(_ query: Query) async {
        isLoading = true
        defer { isLoading = false }
        collapse()
        do {
            let graph = try await client.getGraph(scrape: query.scrape, options: options(query))
            guard !Task.isCancelled else { return }
            self.graph = graph
            loaded = query
            error = nil
            isLayingOut = true
            defer { isLayingOut = false }
            var isFirst = true
            for await positions in Self.layout(nodes: graph.nodes ?? [], edges: graph.edges ?? []) {
                guard !Task.isCancelled else { return }
                self.positions = positions
                if isFirst {
                    isFirst = false
                    layoutID = UUID()
                }
            }
        } catch is CancellationError {
            return
        } catch {
            isLayingOut = false
            self.error = error.localizedDescription
        }
    }

    /// expand loads the pages of a folder and lays them out around it
    func expand(_ folder: Int) async {
        guard expanded != folder, let loaded, let nodes = graph?.nodes, nodes.indices.contains(folder) else { return }
        collapse()
        expanded = folder
        isExpanding = true
        defer { isExpanding = false }
        do {
            let pages = try await client.getFolderPages(scrape: loaded.scrape, options: options(loaded), folder: nodes[folder].id, maxPages: 200)
            guard !Task.isCancelled, expanded == folder else { return }
            let offsets = await Self.satellites(pages: pages.pages ?? [], edges: pages.edges ?? [])
            guard !Task.isCancelled, expanded == folder else { return }
            self.pages = pages
            pageOffsets = offsets
            expansionID = UUID()
        } catch is CancellationError {
            return
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// loadPageLinks loads the links of a page of the expanded folder to the folders
    func loadPageLinks(_ page: Int) async {
        guard linkedPage != page, let loaded, let pages = pages?.pages, pages.indices.contains(page) else { return }
        clearPageLinks()
        linkedPage = page
        do {
            let links = try await client.getPageFolderLinks(scrape: loaded.scrape, options: options(loaded), page: pages[page].id) ?? []
            guard !Task.isCancelled, linkedPage == page else { return }
            pageLinks = links
            pageLinksID = UUID()
        } catch is CancellationError {
            return
        } catch {
            self.error = error.localizedDescription
        }
    }

    func clearPageLinks() {
        guard linkedPage != nil else { return }
        linkedPage = nil
        pageLinks = []
        pageLinksID = UUID()
    }

    func collapse() {
        clearPageLinks()
        guard expanded != nil else { return }
        expanded = nil
        pages = nil
        pageOffsets = []
        expansionID = UUID()
    }

    /// sizes of the nodes in 0...1 - the area grows with the pages linking to
    /// a node (radius ~ sqrt) up to the 95th percentile; the few nodes above it
    /// grow on slowly, so they stay distinguishable without shrinking all
    /// others to dots
    nonisolated static func sizes(of nodes: [GraphNode]) -> [Float] {
        let counts = nodes.map { Float($0.linkedFrom) }
        let sorted = counts.sorted()
        guard let last = sorted.last, last > 0 else { return counts.map { _ in 0 } }
        let cap = max(sorted[min(Int(Float(sorted.count) * 0.95), sorted.count - 1)], 1)
        // above the cap up to 1.4 for the most linked node, logarithmically
        let headroom: Float = 0.4
        let top = last > cap ? log2(last / cap) : 1
        return counts.map { count in
            let size = count <= cap ? (count / cap).squareRoot() : 1 + headroom * log2(count / cap) / top
            return size / (1 + headroom)
        }
    }

    /// layout edges - strong links pull harder, weights in 0.2 ... 1
    nonisolated private static func layoutEdges(_ edges: [GraphEdge], shift: Int = 0) -> [LayoutEdge] {
        let maxLinks = Float(edges.map(\.links).max() ?? 1)
        return edges.map {
            LayoutEdge(from: $0.from + shift, to: $0.to + shift, weight: 0.2 + 0.8 * log1p(Float($0.links)) / log1p(maxLinks))
        }
    }

    /// layout runs the force simulation off the main actor and streams the
    /// positions while it settles - the graph shows up right away and unfolds
    nonisolated static func layout(nodes: [GraphNode], edges: [GraphEdge]) -> AsyncStream<[SIMD3<Float>]> {
        // layout units - the links are 30 long
        let radii = sizes(of: nodes).map { 2 + 10 * $0 }
        let layoutEdges = layoutEdges(edges)
        return AsyncStream { continuation in
            let task = Task.detached(priority: .userInitiated) {
                simulate(radii: radii, edges: layoutEdges) { positions in
                    guard !Task.isCancelled else { return false }
                    continuation.yield(positions)
                    return true
                }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }

    /// satellites lays out the pages of a folder around it: the folder is a
    /// hub linked to every page, links between the pages pull them together;
    /// the offsets are relative to the folder, in a radius that grows with
    /// the number of pages
    nonisolated static func satellites(pages: [GraphNode], edges: [GraphEdge]) async -> [SIMD3<Float>] {
        guard !pages.isEmpty else { return [] }
        let radii = [Float(4)] + sizes(of: pages).map { 1 + 4 * $0 }
        let spokes = pages.indices.map { LayoutEdge(from: 0, to: $0 + 1, weight: 0.3) }
        let layoutEdges = spokes + layoutEdges(edges, shift: 1)
        let radius = min(0.06 + 0.012 * Float(pages.count).squareRoot(), 0.25)
        return await Task.detached(priority: .userInitiated) {
            var last: [SIMD3<Float>] = []
            simulate(radii: radii, edges: layoutEdges) { positions in
                last = positions
                return !Task.isCancelled
            }
            guard let hub = last.first else { return [] }
            return last.dropFirst().map { ($0 - hub) * radius }
        }.value
    }
}
