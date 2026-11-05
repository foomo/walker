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
        // the shape developer of a server - a shape and a website side by side
        WindowGroup("Shape Developer", id: ShapeDevWindow.id, for: Server.ID.self) { $serverID in
            ShapeDevWindow(serverID: serverID)
                .environment(store)
                .environment(appDelegate.walkers)
        }
        Settings {
            SettingsView()
        }
    }
}
