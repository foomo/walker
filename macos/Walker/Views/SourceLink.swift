import AppKit
import SwiftUI
import UniformTypeIdentifiers

/// Editor opens shape files at a line through its url scheme - client and
/// server share a file system, so the shape root is a local path
enum Editor: String, CaseIterable, Identifiable {
    case zed = "Zed"
    case vscode = "VS Code"
    case cursor = "Cursor"
    /// the default app for html files - without the line
    case system = "Default App"

    static let key = "shapeDevEditor"

    var id: String { rawValue }

    /// app is the application that opens this editor's links - nil if none is installed
    @MainActor var app: URL? {
        switch self {
        case .zed: NSWorkspace.shared.urlForApplication(toOpen: URL(string: "zed://")!)
        case .vscode: NSWorkspace.shared.urlForApplication(toOpen: URL(string: "vscode://")!)
        case .cursor: NSWorkspace.shared.urlForApplication(toOpen: URL(string: "cursor://")!)
        case .system: NSWorkspace.shared.urlForApplication(toOpen: .html)
        }
    }

    /// icon is the menu sized icon of the app - nil if it is not installed
    @MainActor var icon: NSImage? {
        guard let app else { return nil }
        let image = NSWorkspace.shared.icon(forFile: app.path(percentEncoded: false)).copy() as? NSImage
        image?.size = NSSize(width: 16, height: 16)
        return image
    }

    /// title names the editor, and for the default app the app it is
    @MainActor var title: String {
        guard self == .system, let app else { return rawValue }
        return "\(rawValue) (\(app.deletingPathExtension().lastPathComponent))"
    }

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

/// EditorMenu picks the editor source links open shape files in, at the line -
/// the same setting as in the settings window. Editors that are not installed
/// can't be picked.
struct EditorMenu: View {
    @AppStorage(Editor.key) private var editor = Editor.zed

    var body: some View {
        Menu {
            Picker("Open Shape Files In", selection: $editor) {
                ForEach(Editor.allCases) { option in
                    EditorLabel(editor: option)
                        .tag(option)
                        .disabled(option.app == nil)
                }
            }
            .pickerStyle(.inline)
        } label: {
            EditorLabel(editor: editor)
                .labelStyle(.titleAndIcon)
        }
        .help(editor.app == nil
            ? "\(editor.rawValue) is not installed - choose the editor shape files open in"
            : "Source links open shape files in \(editor.title), at the line")
    }
}

/// EditorLabel shows an editor with its app icon - or a warning, if it is not installed
struct EditorLabel: View {
    let editor: Editor

    var body: some View {
        Label {
            Text(editor.app == nil ? "\(editor.rawValue) – not installed" : editor.title)
        } icon: {
            if let icon = editor.icon {
                Image(nsImage: icon)
            } else {
                Image(systemName: "exclamationmark.triangle")
            }
        }
    }
}

/// SourceLink is a shape source - file.semanticshape.html:line, relative to the shape root
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

/// ShapeRootKey hands the shape root down to the source links
private struct ShapeRootKey: EnvironmentKey {
    static let defaultValue = ""
}

extension EnvironmentValues {
    var shapeRoot: String {
        get { self[ShapeRootKey.self] }
        set { self[ShapeRootKey.self] = newValue }
    }
}
