import SwiftUI

struct SettingsView: View {
    static let refreshIntervalKey = "refreshInterval"
    static let defaultRefreshInterval = 2.0

    @AppStorage(refreshIntervalKey) private var refreshInterval = defaultRefreshInterval
    @AppStorage(Editor.key) private var editor = Editor.zed
    @AppStorage(LocalWalkers.logLinesKey) private var logLines = LocalWalkers.defaultLogLines

    var body: some View {
        Form {
            Stepper(value: $refreshInterval, in: 1...60, step: 1) {
                LabeledContent("Refresh interval", value: "\(Int(refreshInterval)) s")
            }
            Picker("Shape editor", selection: $editor) {
                ForEach(Editor.allCases) { editor in
                    EditorLabel(editor: editor)
                        .tag(editor)
                        .disabled(editor.app == nil)
                }
            }
            .help("Source links of the shape developer open in this editor, at the line")
            LabeledContent("Log lines per walker") {
                HStack {
                    TextField("Log lines per walker", value: $logLines, format: .number)
                        .labelsHidden()
                        .multilineTextAlignment(.trailing)
                        .frame(width: 90)
                    Stepper("Log lines per walker", value: $logLines, in: 100...1_000_000, step: 1000)
                        .labelsHidden()
                }
            }
            .help("How many lines the log of a local walker keeps - older ones are dropped, at least 100")
        }
        .formStyle(.grouped)
        .frame(width: 420)
    }
}
