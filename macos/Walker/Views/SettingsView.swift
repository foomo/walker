import SwiftUI

struct SettingsView: View {
    static let refreshIntervalKey = "refreshInterval"
    static let defaultRefreshInterval = 2.0

    @AppStorage(refreshIntervalKey) private var refreshInterval = defaultRefreshInterval

    var body: some View {
        Form {
            Stepper(value: $refreshInterval, in: 1...60, step: 1) {
                LabeledContent("Refresh interval", value: "\(Int(refreshInterval)) s")
            }
        }
        .formStyle(.grouped)
        .frame(width: 360)
    }
}
