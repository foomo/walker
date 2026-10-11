import SwiftUI

struct ContentView: View {
    @Environment(ServerStore.self) private var store
    @State private var selection: Server.ID?
    @State private var isAddingServer = false

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
                ForEach(store.servers) { server in
                    ServerRow(model: store.model(for: server))
                        .tag(server.id)
                        .contextMenu {
                            Button("Remove", systemImage: "trash", role: .destructive) {
                                store.remove(server.id)
                            }
                        }
                }
            }
            .navigationSplitViewColumnWidth(min: Columns.serverSidebar.min, ideal: Columns.serverSidebar.ideal, max: Columns.serverSidebar.max)
            .toolbar {
                Button("Add Server", systemImage: "plus") {
                    isAddingServer = true
                }
            }
        } detail: {
            if let server = store.servers.first(where: { $0.id == selection }) {
                ServerDetailView(model: store.model(for: server))
                    .id(server.id)
            } else {
                ContentUnavailableView(
                    "No Server Selected",
                    systemImage: "server.rack",
                    description: Text("Select a walker server or add one.")
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
}

struct ServerRow: View {
    let model: ServerModel

    var body: some View {
        Label {
            VStack(alignment: .leading) {
                Text(model.server.name)
                Text(model.server.baseURL.absoluteString)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        } icon: {
            Circle()
                .fill(color)
                .frame(width: 8, height: 8)
        }
    }

    private var color: Color {
        switch model.connection {
        case .unknown: .gray
        case .online: model.current != nil ? .green : .blue
        case .offline: .red
        }
    }
}
