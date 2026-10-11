import SwiftUI

struct AddServerSheet: View {
    let onAdd: (Server) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var address = "http://localhost:3001"

    private var url: URL? {
        guard let url = URL(string: address.trimmingCharacters(in: .whitespaces)),
              let scheme = url.scheme, ["http", "https"].contains(scheme),
              url.host() != nil
        else { return nil }
        return url
    }

    var body: some View {
        Form {
            TextField("Name", text: $name, prompt: Text("Production"))
            TextField("Address", text: $address, prompt: Text(verbatim: "http://localhost:3001"))
            if url == nil {
                Text("Enter an http or https address of a walker server.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .frame(width: 420)
        .navigationTitle("Add Server")
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("Cancel") { dismiss() }
            }
            ToolbarItem(placement: .confirmationAction) {
                Button("Add") {
                    guard let url else { return }
                    let trimmed = name.trimmingCharacters(in: .whitespaces)
                    onAdd(Server(name: trimmed.isEmpty ? (url.host() ?? address) : trimmed, baseURL: url))
                    dismiss()
                }
                .disabled(url == nil)
            }
        }
    }
}
