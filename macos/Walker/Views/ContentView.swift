import SwiftUI

struct ContentView: View {
    @Environment(ServerStore.self) private var store
    @Environment(LocalWalkers.self) private var walkers
    @State private var selection: Server.ID?
    @State private var isAddingServer = false

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
                // local configs, started by the app - and the walkers it only connects to
                if !store.localServers.isEmpty {
                    Section("Local") {
                        ForEach(store.localServers) { server in
                            ServerRow(model: store.model(for: server))
                                .tag(server.id)
                                .contextMenu { contextMenu(server) }
                        }
                    }
                }
                Section("Remote") {
                    ForEach(store.remoteServers) { server in
                        ServerRow(model: store.model(for: server))
                            .tag(server.id)
                            .contextMenu { contextMenu(server) }
                    }
                }
            }
            .navigationSplitViewColumnWidth(min: Columns.serverSidebar.min, ideal: Columns.serverSidebar.ideal, max: Columns.serverSidebar.max)
            .toolbar {
                Menu("Add", systemImage: "plus") {
                    Button("Add Server…", systemImage: "server.rack") {
                        isAddingServer = true
                    }
                    Button("Add Local Config…", systemImage: "doc.badge.plus") {
                        addLocalConfigs()
                    }
                }
            }
        } detail: {
            if let server = store.server(selection) {
                ServerDetailView(model: store.model(for: server))
                    .id(server.id)
            } else {
                ContentUnavailableView(
                    "No Server Selected",
                    systemImage: "server.rack",
                    description: Text("Select a walker server, add one, or add a local config.")
                )
            }
        }
        .sheet(isPresented: $isAddingServer) {
            AddServerSheet { server in
                store.add(server)
                selection = server.id
            }
        }
        .onAppear {
            if selection == nil {
                selection = store.servers.first?.id
            }
        }
    }

    /// addLocalConfigs adds existing configs or a new one, created in the file browser
    private func addLocalConfigs() {
        let usedPorts = Set(store.localServers.compactMap(\.baseURL.port))
        for url in LocalConfigPanel.choose(usedPorts: usedPorts) {
            selection = store.addLocal(configURL: url).id
        }
    }

    @ViewBuilder
    private func contextMenu(_ server: Server) -> some View {
        if server.isLocal {
            LocalWalkerButton(server: server)
            Button("Show in Finder", systemImage: "folder") {
                showInFinder(server)
            }
            Divider()
        }
        Button("Remove", systemImage: "trash", role: .destructive) {
            walkers.stop(server.id)
            store.remove(server.id)
        }
    }
}

struct ServerRow: View {
    let model: ServerModel

    @Environment(LocalWalkers.self) private var walkers

    private var server: Server { model.server }

    var body: some View {
        HStack {
            Label {
                VStack(alignment: .leading) {
                    Text(server.name)
                    Text(subtitle)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                }
            } icon: {
                Circle()
                    .fill(color)
                    .frame(width: 8, height: 8)
            }
            if server.isLocal {
                Spacer()
                LocalWalkerButton(server: server)
                    .labelStyle(.iconOnly)
                    .buttonStyle(.borderless)
            }
        }
        .help(server.configPath ?? server.baseURL.absoluteString)
    }

    private var subtitle: String {
        guard let url = server.configURL else { return server.baseURL.absoluteString }
        let port = server.baseURL.port.map { ":\($0)" } ?? ""
        return "\(url.lastPathComponent) · \(port)"
    }

    private var color: Color {
        if server.isLocal {
            switch walkers.state(server.id) {
            case .stopped: return .gray
            case .starting, .stopping: return .orange
            case .failed: return .red
            case .running: break
            }
        }
        switch model.connection {
        case .unknown: return .gray
        case .online: return model.current != nil ? .green : .blue
        case .offline: return .red
        }
    }
}
