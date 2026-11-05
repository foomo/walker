import SwiftUI

@main
struct WalkerApp: App {
    @NSApplicationDelegateAdaptor private var appDelegate: AppDelegate

    private var store: ServerStore { appDelegate.store }

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(store)
                .environment(appDelegate.walkers)
        }
        Settings {
            SettingsView()
        }
    }
}
