// swift-tools-version: 5.9
import PackageDescription

// GraphLayout wraps the force simulation of Grape. Grape is generic and
// @inlinable, so its hot loops are compiled in the calling module - in an
// unoptimized debug build of the app the layout is about 100x slower. This
// package is always built optimized, whatever configuration the app uses.
let package = Package(
    name: "GraphLayout",
    platforms: [.macOS(.v14)],
    products: [
        .library(name: "GraphLayout", targets: ["GraphLayout"]),
    ],
    dependencies: [
        // main after 1.1.0 - kinetics.position is public since then, there is no release with it yet
        .package(url: "https://github.com/swiftgraphs/Grape", revision: "bdfad750d2a973af48159119eaf90530d0e30ea7"),
    ],
    targets: [
        .target(
            name: "GraphLayout",
            dependencies: [.product(name: "ForceSimulation", package: "Grape")],
            // only -O: -wmo breaks the per-file outputs Xcode expects in debug
            // builds - with a single source file -O specializes just as well
            swiftSettings: [.unsafeFlags(["-O"])]
        ),
    ]
)
