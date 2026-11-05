import AppKit
import SwiftUI

/// Editor opens schema files at a line through its url scheme - client and
/// server share a file system, so the schema root is a local path
enum Editor: String, CaseIterable, Identifiable {
    case zed = "Zed"
    case vscode = "VS Code"
    case cursor = "Cursor"
    /// the default app for html files - without the line
    case system = "Default App"

    static let key = "schemaDevEditor"

    var id: String { rawValue }

    func url(path: String, line: Int?) -> URL? {
        let position = line.map { ":\($0)" } ?? ""
        var encoded = path.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? path
        if !encoded.hasPrefix("/") {
            encoded = "/" + encoded
        }
        switch self {
        case .zed:
            return URL(string: "zed://file" + encoded + position)
        case .vscode:
            return URL(string: "vscode://file" + encoded + position)
        case .cursor:
            return URL(string: "cursor://file" + encoded + position)
        case .system:
            return URL(filePath: path)
        }
    }
}

/// SourceLink is a schema source - file.shape.html:line, relative to the schema root
/// - that opens in the editor of the settings
struct SourceLink: View {
    let root: String
    let source: String
    var text: String?
    @AppStorage(Editor.key) private var editor = Editor.zed

    var body: some View {
        Button {
            open()
        } label: {
            Text(text ?? source)
                .underline()
        }
        .buttonStyle(.plain)
        .foregroundStyle(Color.accentColor)
        .pointerStyle(.link)
        .help("Open in \(editor.rawValue)")
        .disabled(root.isEmpty || source.isEmpty)
    }

    private func open() {
        let file = TraceSource.file(source)
        let line = source.count > file.count ? Int(source.dropFirst(file.count + 1)) : nil
        let path = (root as NSString).appendingPathComponent(file)
        if let url = editor.url(path: path, line: line) {
            NSWorkspace.shared.open(url)
        }
    }
}

/// SchemaRootKey hands the schema root down to the source links
private struct SchemaRootKey: EnvironmentKey {
    static let defaultValue = ""
}

extension EnvironmentValues {
    var schemaRoot: String {
        get { self[SchemaRootKey.self] }
        set { self[SchemaRootKey.self] = newValue }
    }
}
