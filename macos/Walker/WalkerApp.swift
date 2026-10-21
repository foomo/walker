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
        // the schema developer of a server - a schema and a website side by side
        WindowGroup("Schema Developer", id: SchemaDevWindow.id, for: Server.ID.self) { $serverID in
            SchemaDevWindow(serverID: serverID)
                .environment(store)
                .environment(appDelegate.walkers)
        }
        Settings {
            SettingsView()
        }
    }
}
