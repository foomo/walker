import ForceSimulation
import Foundation
import simd

/// LayoutEdge pulls two nodes together - weight in 0...1 scales the stiffness.
public struct LayoutEdge: Sendable {
    public let from: Int
    public let to: Int
    public let weight: Float

    public init(from: Int, to: Int, weight: Float) {
        self.from = from
        self.to = to
        self.weight = weight
    }
}

/// simulate lays out nodes in 3D - links pull, nodes repel and collide by
/// their radius. report gets the positions, centered and scaled into a radius
/// of 1, about every 100ms and at the end - it returns false to stop.
public func simulate(radii: [Float], edges: [LayoutEdge], ticks: Int = 300, report: ([SIMD3<Float>]) -> Bool) {
    let count = radii.count
    guard count > 1 else {
        _ = report(count == 1 ? [.zero] : [])
        return
    }
    var weights: [Int: [Int: Float]] = [:]
    for edge in edges {
        weights[edge.from, default: [:]][edge.to] = edge.weight
    }

    // a fibonacci sphere as the start - coincident nodes give the forces nothing to work with
    let spread = 10 * Float(count).squareRoot()
    let golden = Float.pi * (3 - Float(5).squareRoot())
    let start: [SIMD3<Float>] = (0..<count).map { i in
        let y = 1 - 2 * (Float(i) + 0.5) / Float(count)
        let r = (1 - y * y).squareRoot()
        let theta = golden * Float(i)
        return SIMD3(cos(theta) * r, y, sin(theta) * r) * spread
    }

    let force = SealedForce3D([
        .link(Kinetics3D.LinkForce(
            stiffness: .weightedByDegree { edge, _ in weights[edge.source]?[edge.target] ?? 0.2 },
            originalLength: .constant(30)
        )),
        .manyBody(Kinetics3D.ManyBodyForce(strength: -40)),
        .center(Kinetics3D.CenterForce(center: .zero, strength: 1)),
        .collide(Kinetics3D.CollideForce(radius: .varied { radii[$0] * 2 })),
    ])
    let links = edges.map { EdgeID(source: $0.from, target: $0.to) }
    let simulation = Simulation3D(nodeCount: count, links: links, forceField: force, position: start)
    var lastReport = ContinuousClock.now
    for tick in 1...max(ticks, 1) {
        simulation.tick()
        let now = ContinuousClock.now
        if tick == ticks || now - lastReport > .milliseconds(100) {
            lastReport = now
            if !report(normalized(simulation.kinetics.position.asArray())) {
                return
            }
        }
    }
}

/// normalized centers the positions and scales them into a radius of 1
func normalized(_ positions: [SIMD3<Float>]) -> [SIMD3<Float>] {
    guard !positions.isEmpty else { return [] }
    let center = positions.reduce(SIMD3<Float>.zero, +) / Float(positions.count)
    let extent = positions.map { simd_length($0 - center) }.max() ?? 0
    let scale = extent > 0 ? 1 / extent : 1
    return positions.map { ($0 - center) * scale }
}
