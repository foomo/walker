import AppKit
import UniformTypeIdentifiers

/// LocalConfigPanel is the file browser of "Add Local Config…": it opens
/// existing walker configs, its "New Config…" button creates one instead
@MainActor
enum LocalConfigPanel {
    /// choose returns the configs to add - the chosen ones or the one just
    /// created - empty when the user cancels. usedPorts are the ports of the
    /// local configs, a new config listens on the next free one.
    static func choose(usedPorts: Set<Int>) -> [URL] {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.yaml]
        panel.allowsMultipleSelection = true
        panel.canChooseDirectories = false
        panel.prompt = "Add"
        panel.message = "Choose walker configs, or create a new one"

        let newConfig = NewConfigAction(panel: panel)
        let button = NSButton(title: "New Config…", target: newConfig, action: #selector(NewConfigAction.run))
        button.translatesAutoresizingMaskIntoConstraints = false
        let accessory = NSView()
        accessory.addSubview(button)
        NSLayoutConstraint.activate([
            button.centerXAnchor.constraint(equalTo: accessory.centerXAnchor),
            button.topAnchor.constraint(equalTo: accessory.topAnchor, constant: 8),
            button.bottomAnchor.constraint(equalTo: accessory.bottomAnchor, constant: -8),
        ])
        panel.accessoryView = accessory
        panel.isAccessoryViewDisclosed = true

        let response = panel.runModal()
        if newConfig.requested {
            guard let url = create(in: newConfig.directory, usedPorts: usedPorts) else { return [] }
            return [url]
        }
        return response == .OK ? panel.urls : []
    }

    /// create asks for the name of the new config and writes the template
    private static func create(in directory: URL?, usedPorts: Set<Int>) -> URL? {
        let panel = NSSavePanel()
        panel.allowedContentTypes = [.yaml]
        panel.directoryURL = directory
        panel.nameFieldStringValue = "walker.yaml"
        panel.prompt = "Create"
        panel.message = "Create a new walker config"
        guard panel.runModal() == .OK, let url = panel.url else { return nil }
        do {
            try template(port: freePort(usedPorts)).write(to: url, atomically: true, encoding: .utf8)
            return url
        } catch {
            NSAlert(error: error).runModal()
            return nil
        }
    }

    /// freePort is the first port from walker's default 3001 on that no local config uses
    private static func freePort(_ used: Set<Int>) -> Int {
        var port = 3001
        while used.contains(port) {
            port += 1
        }
        return port
    }

    /// template is a new config: the defaults spelled out, the target still to be set
    private static func template(port: Int) -> String {
        """
        # walker config - edit it in the overview of the local server
        # the site to crawl, a path in the url is the start path
        target: https://www.example.com/
        # parallel requests - high values can take a site down
        concurrency: 2
        # address of the web ui, the service and /metrics
        addr: ":\(port)"
        usecookies: true
        agent: foomo-walker

        """
    }
}

/// NewConfigAction ends the open panel when its button is pressed and keeps
/// the directory the panel showed, the new config goes there
@MainActor
private final class NewConfigAction: NSObject {
    private weak var panel: NSOpenPanel?
    private(set) var requested = false
    private(set) var directory: URL?

    init(panel: NSOpenPanel) {
        self.panel = panel
    }

    @objc func run() {
        requested = true
        directory = panel?.directoryURL
        panel?.cancel(nil)
    }
}
