import Foundation

/// ConfigTool runs the configvalidator helper bundled in Contents/Helpers -
/// the yaml backend of the config form, the same code as walker uses
enum ConfigTool {
    struct Problem: Codable, Hashable, Sendable {
        let severity: String
        let key: String
        let line: Int
        let column: Int
        let message: String
    }

    /// Form mirrors config.Form of the go side
    struct Form: Codable, Equatable, Sendable {
        var baseURL = ""
        var paths: [String] = []
        var concurrency = 2
        var addr = ":3001"
        var agent = "foomo-walker"
        var useCookies = true
        var ignore: [String] = []
        var ignoreQueriesWith: [String] = []
        var ignoreAllQueries = false
        var ignoreRobots = false
        var depth = 0
        var paging = false
        var groupHeader = ""
        var schemaRoot = ""
        var interval = ""
        var offset = ""
    }

    struct ReadResult: Codable, Sendable {
        let form: Form
        let problems: [Problem]
    }

    enum Failure: LocalizedError {
        case missingHelper
        case failed(String)

        var errorDescription: String? {
            switch self {
            case .missingHelper:
                "The configvalidator helper is missing - build the app with Go installed."
            case let .failed(message):
                message
            }
        }
    }

    static var helperURL: URL {
        Bundle.main.bundleURL.appending(path: "Contents/Helpers/configvalidator")
    }

    /// read parses the yaml into the form and validates it - base is the
    /// directory relative paths of the config are resolved against
    static func read(yaml: String, base: String) async throws -> ReadResult {
        var arguments = ["read"]
        if !base.isEmpty {
            arguments += ["-base", base]
        }
        let data = try await run(arguments + ["-"], input: Data(yaml.utf8))
        return try JSONDecoder().decode(ReadResult.self, from: data)
    }

    /// write applies the form to the yaml - comments and unknown keys stay
    static func write(yaml: String, form: Form) async throws -> String {
        struct Request: Encodable {
            let yaml: String
            let form: Form
        }
        let input = try JSONEncoder().encode(Request(yaml: yaml, form: form))
        let data = try await run(["write", "-"], input: input)
        return String(decoding: data, as: UTF8.self)
    }

    private static func run(_ arguments: [String], input: Data) async throws -> Data {
        let url = helperURL
        guard FileManager.default.isExecutableFile(atPath: url.path(percentEncoded: false)) else {
            throw Failure.missingHelper
        }
        return try await Task.detached {
            let process = Process()
            process.executableURL = url
            process.arguments = arguments
            let stdin = Pipe(), stdout = Pipe(), stderr = Pipe()
            process.standardInput = stdin
            process.standardOutput = stdout
            process.standardError = stderr
            try process.run()
            // the helper reads all of its input before it writes
            try stdin.fileHandleForWriting.write(contentsOf: input)
            try stdin.fileHandleForWriting.close()
            let output = stdout.fileHandleForReading.readDataToEndOfFile()
            let errors = stderr.fileHandleForReading.readDataToEndOfFile()
            process.waitUntilExit()
            // 3 means the config has errors - they are in the output
            guard process.terminationStatus == 0 || process.terminationStatus == 3 else {
                throw Failure.failed(String(decoding: errors, as: UTF8.self))
            }
            return output
        }.value
    }
}
