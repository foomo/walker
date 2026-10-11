import RealityKit
import SwiftUI

/// GraphView shows the folders of the site as a 3D graph - drag to orbit,
/// scroll to zoom, click a folder to expand its pages and inspect it.
struct GraphView: View {
    let model: ServerModel
    @State private var selection: GraphModel.Selection?
    @State private var scene = GraphScene()
    @State private var showsGlobal = false
    @AppStorage("graphShowsLabels") private var showsLabels = true

    private var graph: GraphModel { model.graph }

    var body: some View {
        RealityView { content in
            content.add(scene.root)
        } update: { _ in
            scene.update(graph: graph, colors: colors, selection: selection, showsLabels: showsLabels)
        }
        .realityViewCameraControls(.orbit)
        .gesture(
            SpatialTapGesture()
                .targetedToAnyEntity()
                .onEnded { value in
                    selection = scene.selection(of: value.entity, graph: graph)
                }
        )
        .background(.black.opacity(0.85))
        .overlay {
            if graph.isLoading && graph.positions.isEmpty {
                ProgressView("Loading …")
                    .padding()
                    .background(.regularMaterial, in: .rect(cornerRadius: 10))
            } else if let error = graph.error {
                ContentUnavailableView("Could Not Load the Graph", systemImage: "exclamationmark.triangle", description: Text(error))
            } else if (graph.graph?.nodes ?? []).isEmpty {
                ContentUnavailableView("No Pages", systemImage: "point.3.connected.trianglepath.dotted", description: Text("This scrape has no pages yet."))
            }
        }
        .overlay(alignment: .bottomLeading) {
            // the graph unfolds while the layout settles
            if graph.isLayingOut || graph.isLoading || graph.isExpanding {
                HStack(spacing: 6) {
                    ProgressView()
                        .controlSize(.small)
                    Text(graph.isLoading ? "Loading …" : graph.isExpanding ? "Loading pages …" : "Laying out …")
                        .font(.caption)
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 6)
                .background(.regularMaterial, in: .capsule)
                .padding(10)
            }
        }
        .overlay(alignment: .topLeading) {
            if let node = selectedNode {
                NodeCaption(node: node, dropped: isFolderSelected ? graph.pages?.dropped ?? 0 : 0)
                    .padding(10)
            }
        }
        .safeAreaInset(edge: .top, spacing: 0) {
            controls
        }
        .inspector(isPresented: Binding(get: { selection != nil }, set: { if !$0 { selection = nil } })) {
            Group {
                if let node = selectedNode {
                    if isFolderSelected {
                        GraphNodeInspector(node: node)
                    } else {
                        PageLinksInspector(url: node.id, model: model)
                            .id(node.id)
                    }
                }
            }
            .inspectorColumnWidth(min: Columns.inspector.min, ideal: Columns.inspector.ideal, max: Columns.inspector.max)
        }
        .toolbar {
            ToolbarItem {
                Button("Reload", systemImage: "arrow.clockwise") {
                    Task { await graph.load(query) }
                }
            }
        }
        .task(id: query) {
            selection = nil
            await graph.load(query)
        }
        .task(id: selection) {
            // a folder expands into its pages - selecting one of them keeps it open
            switch selection {
            case let .folder(folder):
                graph.clearPageLinks()
                await graph.expand(folder)
            case let .page(page):
                // where the page links to among the folders
                await graph.loadPageLinks(page)
            case nil:
                graph.collapse()
            }
        }
    }

    private var query: GraphModel.Query {
        graph.query(scrape: model.scrape, completed: model.complete?.completed)
    }

    private var isFolderSelected: Bool {
        if case .folder = selection { true } else { false }
    }

    private var selectedNode: GraphNode? {
        switch selection {
        case let .folder(index):
            let nodes = graph.graph?.nodes ?? []
            return nodes.indices.contains(index) ? nodes[index] : nil
        case let .page(index):
            let pages = graph.pages?.pages ?? []
            return pages.indices.contains(index) ? pages[index] : nil
        case nil:
            return nil
        }
    }

    /// colors of the folders, then the pages of the expanded folder
    private var colors: [Color] {
        let nodes = (graph.graph?.nodes ?? []) + (graph.pages?.pages ?? [])
        switch graph.colorMode {
        case .status:
            return nodes.map { StatusClass.color(for: $0.code) }
        case .group:
            let groups = Array(Set(nodes.map(\.group))).sorted()
            return nodes.map { GraphView.palette[(groups.firstIndex(of: $0.group) ?? 0) % GraphView.palette.count] }
        case .performance:
            let buckets = (model.current ?? model.complete)?.mergedBuckets ?? []
            guard !buckets.isEmpty else { return nodes.map { _ in .gray } }
            return nodes.map { BucketShare.color(index: buckets.bucketIndex(of: $0.p90), count: buckets.count) }
        }
    }

    static let palette: [Color] = [.blue, .orange, .green, .pink, .purple, .yellow, .teal, .red, .mint, .indigo, .brown, .cyan]

    private var controls: some View {
        @Bindable var graph = graph
        return HStack(spacing: 12) {
            Stepper("Folder depth \(graph.depth)", value: $graph.depth, in: 1...6)
                .fixedSize()
            Toggle("Labels", isOn: $showsLabels)
                .toggleStyle(.checkbox)
                .fixedSize()
            Picker("Color", selection: $graph.colorMode) {
                ForEach(GraphModel.ColorMode.allCases) { mode in
                    Text(mode.rawValue).tag(mode)
                }
            }
            .fixedSize()
            Spacer()
            if let result = graph.graph {
                Text("\((result.nodes ?? []).count) folders · \((result.edges ?? []).count) edges")
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
                if !(result.global ?? []).isEmpty {
                    Button("\((result.global ?? []).count) navigation links hidden") {
                        showsGlobal = true
                    }
                    .buttonStyle(.link)
                    .popover(isPresented: $showsGlobal) {
                        GlobalLinksList(links: result.global ?? [])
                    }
                }
            }
        }
        .font(.callout)
        .padding(.horizontal)
        .padding(.vertical, 8)
        .background(.bar)
    }
}

/// GraphScene owns the RealityKit entities. Folders and the pages of the
/// expanded folder are one list of nodes - folders first - so that light,
/// labels and edges treat them alike.
@MainActor
final class GraphScene {
    let root = Entity()
    private let nodes = Entity()
    private let edges = Entity()
    private let labels = Entity()

    private var builtLayout: UUID?
    private var builtExpansion: UUID?
    private var builtPageLinks: UUID?
    /// edges of the folders, the spokes and the links between the pages - without page links
    private var baseEdges: [GraphEdge] = []
    private var builtFolderPositions: [SIMD3<Float>] = []
    private var builtColors: [Color] = []
    private var builtSelection: Int?
    private var builtLabels: (shown: Bool, selection: Int?)?

    /// folders is the number of folder nodes - pages follow them
    private var folders = 0
    private var expanded: Int?
    private var pageOffsets: [SIMD3<Float>] = []
    /// positions of all nodes, normalized into a radius of about 1
    private var positions: [SIMD3<Float>] = []
    private var edgeList: [GraphEdge] = []
    private var nodeEntities: [ModelEntity] = []
    private var radii: [Float] = []
    private var labelTexts: [String] = []
    private var labelEntities: [Int: Entity] = [:]
    /// labeled are the nodes, that get a label - the largest ones
    private var labeled: Set<Int> = []
    /// neighbors of every node, links in both directions
    private var neighbors: [[Int]] = []

    /// light of every node in 0...1 - like a torch at the selected node, it
    /// falls off with the distance in space; nil without a selection
    private var light: [Float]?
    private var selection: Int?
    /// background the dark nodes fade into - the view's background
    private static let darkness = NSColor(white: 0.12, alpha: 1)
    /// brightness steps of the edges - one mesh each
    private static let edgeSteps = 8

    private static let labelMaterial: UnlitMaterial = {
        var material = UnlitMaterial(color: NSColor(white: 0.85, alpha: 1))
        material.blending = .transparent(opacity: 0.55)
        return material
    }()
    private static let selectedLabelMaterial = UnlitMaterial(color: .white)

    /// scale from layout units into the scene - a radius of about 0.5 m
    private static let sceneRadius: Float = 0.5
    private static let namePrefix = "node-"

    init() {
        root.addChild(edges)
        root.addChild(nodes)
        root.addChild(labels)
        // the whole graph - nodes, edges and labels alike - fits the default camera
        root.scale = SIMD3(repeating: 0.6)
    }

    /// torch - full light close to the selection, dark beyond about half the
    /// graph's radius
    private static func torch(_ distance: Float) -> Float {
        let t = min(max((distance - 0.08) / (0.55 - 0.08), 0), 1)
        let falloff = 1 - t * t * (3 - 2 * t)
        return 0.04 + 0.96 * falloff
    }

    /// selection maps a tapped entity to a folder or a page
    func selection(of entity: Entity, graph: GraphModel) -> GraphModel.Selection? {
        guard let index = Int(entity.name.dropFirst(Self.namePrefix.count)) else { return nil }
        return index < folders ? .folder(index) : .page(index - folders)
    }

    private func index(of selection: GraphModel.Selection?) -> Int? {
        switch selection {
        case let .folder(index): index
        case let .page(index): folders + index
        case nil: nil
        }
    }

    func update(graph: GraphModel, colors: [Color], selection: GraphModel.Selection?, showsLabels: Bool) {
        guard let result = graph.graph, graph.positions.count == (result.nodes ?? []).count else { return }
        let selected = index(of: selection)
        if builtLayout != graph.layoutID || builtExpansion != graph.expansionID {
            builtLayout = graph.layoutID
            builtExpansion = graph.expansionID
            build(result, graph: graph)
            builtFolderPositions = graph.positions
            builtColors = []
            builtSelection = nil
            builtLabels = nil
        } else if builtFolderPositions != graph.positions {
            // the layout settles - move the nodes, redraw the edges
            builtFolderPositions = graph.positions
            move(folderPositions: graph.positions)
        }
        if builtPageLinks != graph.pageLinksID {
            builtPageLinks = graph.pageLinksID
            linkPage(graph)
        }
        if builtColors != colors || builtSelection != selected {
            if builtSelection != selected {
                self.selection = selected
                relight()
                drawEdges()
            }
            builtColors = colors
            builtSelection = selected
            paint(colors: colors)
        }
        if builtLabels?.shown != showsLabels || builtLabels?.selection != selected {
            builtLabels = (showsLabels, selected)
            label(showsLabels: showsLabels)
        }
    }

    private func build(_ result: Graph, graph: GraphModel) {
        nodes.children.removeAll()
        labels.children.removeAll()
        labelEntities = [:]
        nodeEntities = []
        light = nil
        selection = nil

        let folderNodes = result.nodes ?? []
        let pageNodes = graph.pages?.pages ?? []
        folders = folderNodes.count
        expanded = graph.expanded
        pageOffsets = graph.pageOffsets.count == pageNodes.count ? graph.pageOffsets : []
        let pages = pageOffsets.isEmpty ? [] : pageNodes

        // folders, then the spokes to the pages and the links between them
        edgeList = result.edges ?? []
        if let expanded {
            edgeList += pages.indices.map { GraphEdge(from: expanded, to: folders + $0, links: 1) }
            edgeList += (graph.pages?.edges ?? []).map { GraphEdge(from: folders + $0.from, to: folders + $0.to, links: $0.links) }
        }
        let count = folders + pages.count
        baseEdges = edgeList
        builtPageLinks = nil
        updateNeighbors(count: count)

        // pages are smaller than folders - both sized by the pages linking to them
        radii = GraphModel.sizes(of: folderNodes).map { 0.002 + 0.016 * $0 }
            + GraphModel.sizes(of: pages).map { 0.0012 + 0.006 * $0 }
        // folders by name, pages by the last segment of their path - the caption has the full one
        labelTexts = folderNodes.map(\.label) + pages.map { page in
            page.label.split(separator: "/").last.map(String.init) ?? "/"
        }
        // the largest get a label - up to 150 folders and 20 pages
        let folderSizes = GraphModel.sizes(of: folderNodes)
        let pageSizes = GraphModel.sizes(of: pages)
        labeled = Set(folderSizes.indices.sorted { folderSizes[$0] > folderSizes[$1] }.prefix(150))
            .union(pageSizes.indices.sorted { pageSizes[$0] > pageSizes[$1] }.prefix(20).map { folders + $0 })

        positions = Self.positions(folders: graph.positions, expanded: expanded, offsets: pageOffsets)
        let sphere = MeshResource.generateSphere(radius: 1)
        for index in 0..<count {
            let entity = ModelEntity(mesh: sphere, materials: [UnlitMaterial()])
            entity.name = Self.namePrefix + String(index)
            entity.position = positions[index] * Self.sceneRadius
            entity.scale = SIMD3(repeating: radii[index])
            entity.components.set(CollisionComponent(shapes: [.generateSphere(radius: 1)]))
            entity.components.set(InputTargetComponent())
            nodes.addChild(entity)
            nodeEntities.append(entity)
        }
        drawEdges()
    }

    private func updateNeighbors(count: Int) {
        neighbors = Array(repeating: [], count: count)
        for edge in edgeList where neighbors.indices.contains(edge.from) && neighbors.indices.contains(edge.to) {
            neighbors[edge.from].append(edge.to)
            neighbors[edge.to].append(edge.from)
        }
    }

    /// linkPage adds the links of the selected page to the folders - the
    /// linked folders become its neighbors and stay lit
    private func linkPage(_ graph: GraphModel) {
        edgeList = baseEdges
        if let page = graph.linkedPage, folders + page < nodeEntities.count {
            edgeList += graph.pageLinks
                .filter { $0.folder < folders }
                .map { GraphEdge(from: folders + page, to: $0.folder, links: $0.links) }
        }
        updateNeighbors(count: nodeEntities.count)
        if selection != nil {
            relight()
            paint(colors: builtColors)
            if let shown = builtLabels {
                label(showsLabels: shown.shown)
            }
        }
        drawEdges()
    }

    /// positions of the folders, then the pages around the expanded folder
    private static func positions(folders: [SIMD3<Float>], expanded: Int?, offsets: [SIMD3<Float>]) -> [SIMD3<Float>] {
        guard let expanded, folders.indices.contains(expanded) else { return folders }
        return folders + offsets.map { folders[expanded] + $0 }
    }

    private func move(folderPositions: [SIMD3<Float>]) {
        positions = Self.positions(folders: folderPositions, expanded: expanded, offsets: pageOffsets)
        for (index, entity) in nodeEntities.enumerated() where positions.indices.contains(index) {
            entity.position = positions[index] * Self.sceneRadius
        }
        for (index, label) in labelEntities {
            label.position = labelPosition(index)
        }
        if selection != nil {
            // the torch moves with the layout
            relight()
            paint(colors: builtColors)
            if let shown = builtLabels {
                label(showsLabels: shown.shown)
            }
        }
        drawEdges()
    }

    /// relight computes the light of the nodes from the selection - direct
    /// neighbors keep some light, so that links stay traceable into the dark
    private func relight() {
        guard let selection, positions.indices.contains(selection) else {
            light = nil
            return
        }
        let center = positions[selection]
        var light = positions.map { Self.torch(simd_length($0 - center)) }
        for neighbor in neighbors[selection] {
            light[neighbor] = max(light[neighbor], 0.5)
        }
        light[selection] = 1
        self.light = light
    }

    private func paint(colors: [Color]) {
        for (index, entity) in nodeEntities.enumerated() {
            let color = colors.indices.contains(index) ? colors[index] : .gray
            // flat and unlit - reads as a clean colored disc from every angle;
            // with a selection the nodes darken away from the torch
            var nsColor = NSColor(color)
            if let light, light.indices.contains(index), light[index] < 1 {
                nsColor = nsColor.blended(withFraction: CGFloat(1 - light[index]), of: Self.darkness) ?? nsColor
            }
            entity.model?.materials = [UnlitMaterial(color: nsColor)]
            entity.scale = SIMD3(repeating: radii[index] * (index == selection ? 1.8 : 1))
        }
    }

    /// label shows the labels of the largest nodes and the selected one -
    /// text meshes are made once per node and kept
    private func label(showsLabels: Bool) {
        var shown = showsLabels ? labeled : []
        if let selection {
            shown.insert(selection)
        }
        for (index, entity) in labelEntities where !shown.contains(index) {
            entity.isEnabled = false
        }
        for index in shown where nodeEntities.indices.contains(index) {
            let entity = labelEntities[index] ?? makeLabel(index)
            entity.isEnabled = true
            // dimmed labels - the selected one stands out
            entity.scale = SIMD3(repeating: index == selection ? 1.5 : index >= folders ? 0.7 : 1)
            var material = index == selection ? Self.selectedLabelMaterial : Self.labelMaterial
            if index != selection, let light, light.indices.contains(index) {
                // labels dim with the light - and vanish in the dark
                entity.isEnabled = light[index] > 0.15
                material.blending = .transparent(opacity: .init(floatLiteral: 0.75 * light[index]))
            }
            (entity.children.first as? ModelEntity)?.model?.materials = [material]
        }
    }

    private func makeLabel(_ index: Int) -> Entity {
        let mesh = MeshResource.generateText(
            labelTexts[index],
            extrusionDepth: 0,
            font: .systemFont(ofSize: 0.008, weight: .regular),
            alignment: .center
        )
        let text = ModelEntity(mesh: mesh, materials: [Self.labelMaterial])
        // center the text on its anchor
        text.position = [-mesh.bounds.center.x, 0, 0]
        let anchor = Entity()
        anchor.addChild(text)
        anchor.components.set(BillboardComponent())
        anchor.position = labelPosition(index)
        labels.addChild(anchor)
        labelEntities[index] = anchor
        return anchor
    }

    /// labelPosition - right above the node
    private func labelPosition(_ index: Int) -> SIMD3<Float> {
        nodeEntities[index].position + [0, radii[index] * 1.8 + 0.006, 0]
    }

    /// drawEdges - one mesh for all edges, or with a selection one per step
    /// of light: an edge is lit at its midpoint, edges of the selection stay bright
    private func drawEdges() {
        edges.children.removeAll()
        let scaled = positions.map { $0 * Self.sceneRadius }
        let maxLinks = Float(edgeList.map(\.links).max() ?? 1)
        var steps: [Int: [GraphEdge]] = [:]
        for edge in edgeList where positions.indices.contains(edge.from) && positions.indices.contains(edge.to) {
            var step = -1
            if let selection, positions.indices.contains(selection) {
                var light = Self.torch(simd_length((positions[edge.from] + positions[edge.to]) / 2 - positions[selection]))
                if edge.from == selection || edge.to == selection {
                    light = max(light, 0.9)
                }
                step = min(Int(light * Float(Self.edgeSteps)), Self.edgeSteps - 1)
            }
            steps[step, default: []].append(edge)
        }
        for (step, stepEdges) in steps {
            guard let mesh = Self.edgeMesh(stepEdges, positions: scaled, maxLinks: maxLinks) else { continue }
            var material = UnlitMaterial(color: NSColor(white: 0.85, alpha: 1))
            // without a selection all edges alike - with one, bright near the torch, gone in the dark
            let light: Float? = step < 0 ? nil : (Float(step) + 0.5) / Float(Self.edgeSteps)
            material.blending = .transparent(opacity: .init(floatLiteral: light.map { 0.02 + 0.6 * $0 * $0 } ?? 0.12))
            edges.addChild(ModelEntity(mesh: mesh, materials: [material]))
        }
    }

    /// edgeMesh merges all edges into one mesh of thin triangular prisms -
    /// one entity instead of thousands
    private static func edgeMesh(_ edges: [GraphEdge], positions: [SIMD3<Float>], maxLinks: Float) -> MeshResource? {
        guard !edges.isEmpty else { return nil }
        var vertices: [SIMD3<Float>] = []
        var indices: [UInt32] = []
        vertices.reserveCapacity(edges.count * 6)
        indices.reserveCapacity(edges.count * 24)
        for edge in edges {
            guard positions.indices.contains(edge.from), positions.indices.contains(edge.to) else { continue }
            let a = positions[edge.from]
            let b = positions[edge.to]
            let axis = b - a
            guard simd_length(axis) > 0 else { continue }
            let direction = simd_normalize(axis)
            let helper: SIMD3<Float> = abs(direction.y) < 0.9 ? [0, 1, 0] : [1, 0, 0]
            let u = simd_normalize(simd_cross(direction, helper))
            let v = simd_cross(direction, u)
            // hairlines - strong connections slightly thicker
            let width: Float = 0.00025 + 0.0009 * log1p(Float(edge.links)) / log1p(maxLinks)
            let corners = [u, -0.5 * u + 0.866 * v, -0.5 * u - 0.866 * v].map { $0 * width }
            let base = UInt32(vertices.count)
            for corner in corners { vertices.append(a + corner) }
            for corner in corners { vertices.append(b + corner) }
            // three sides of the prism, two triangles each, both windings - visible from everywhere
            for side in 0..<3 {
                let i0 = base + UInt32(side)
                let i1 = base + UInt32((side + 1) % 3)
                let j0 = i0 + 3
                let j1 = i1 + 3
                indices += [i0, i1, j1, i0, j1, j0, i0, j1, i1, i0, j0, j1]
            }
        }
        var descriptor = MeshDescriptor(name: "edges")
        descriptor.positions = MeshBuffer(vertices)
        descriptor.primitives = .triangles(indices)
        return try? MeshResource.generate(from: [descriptor])
    }
}

/// NodeCaption names the selected node on top of the graph.
struct NodeCaption: View {
    let node: GraphNode
    /// dropped pages of an expanded folder - only the most linked are shown
    var dropped = 0

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(verbatim: node.label)
                .font(.headline)
            Text("\(node.pages) pages · \(node.linkedFrom) linking pages · \(StatusClass.label(for: node.code))")
                .font(.caption)
                .monospacedDigit()
                .foregroundStyle(.secondary)
            if dropped > 0 {
                Text("showing the \(node.pages - dropped) most linked pages")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .lineLimit(1)
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(.regularMaterial, in: .rect(cornerRadius: 8))
    }
}

/// GraphNodeInspector summarizes a folder of the graph.
struct GraphNodeInspector: View {
    let node: GraphNode

    var body: some View {
        Form {
            Section {
                ResultHeader(title: node.label, description: "", url: node.id)
            }
            Section("Folder") {
                LabeledContent("Pages", value: node.pages.formatted())
                LabeledContent("Errors") {
                    Text(node.errors.formatted())
                        .foregroundStyle(node.errors > 0 ? .orange : .primary)
                }
                LabeledContent("Worst status") {
                    Text(StatusClass.label(for: node.code))
                        .foregroundStyle(StatusClass.color(for: node.code))
                }
                LabeledContent("Group", value: node.group.isEmpty ? "–" : node.group)
                LabeledContent("P90", value: node.p90.formatted)
                LabeledContent("Linking pages", value: node.linkedFrom.formatted())
            }
        }
        .formStyle(.grouped)
        .textSelection(.enabled)
    }
}

/// GlobalLinksList shows the navigation targets left out of the graph.
struct GlobalLinksList: View {
    let links: [GlobalLink]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Linked from more than half of the pages - navigation, not structure")
                .font(.caption)
                .foregroundStyle(.secondary)
            List(links, id: \.url) { link in
                HStack {
                    URLValue(link.url, isLink: true)
                    Spacer()
                    Text(link.linkedFrom.formatted())
                        .monospacedDigit()
                        .foregroundStyle(.secondary)
                }
            }
            .frame(minHeight: 160)
        }
        .padding()
        .frame(width: 440, height: 320)
    }
}
