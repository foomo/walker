import SwiftUI

@main
struct WalkerApp: App {
    @State private var store = ServerStore()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(store)
        }
        Settings {
            SettingsView()
        }
    }
}
