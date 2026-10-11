import SwiftUI

/// FolderInspector shows a folder of the tree and - if it was scraped - the
/// folder's own page, e.g. https://example.com/shop for the folder shop.
struct FolderInspector: View {
    let folder: Folder
    let page: ScrapeResult?
    /// isLoading is true until the results of the folder arrived
    let isLoading: Bool
    let lookup: ResultLookup
    /// open navigates to a sub folder
    let open: (Folder) -> Void

    var body: some View {
        Form {
            Section("Folder") {
                LabeledContent("Prefix") {
                    URLValue(folder.prefix, isLink: true)
                }
                LabeledContent("Pages", value: folder.pages.formatted())
                LabeledContent("Errors") {
                    if folder.errors > 0 {
                        Label(folder.errors.formatted(), systemImage: "exclamationmark.triangle.fill")
                            .foregroundStyle(.orange)
                    } else {
                        Text("none")
                    }
                }
            }

            if let subfolders = folder.folders, !subfolders.isEmpty {
                Section("Subfolders") {
                    ForEach(subfolders, id: \.prefix) { subfolder in
                        Button {
                            open(subfolder)
                        } label: {
                            HStack {
                                FolderLabel(folder: subfolder, isHost: false)
                                Image(systemName: "chevron.right")
                                    .foregroundStyle(.tertiary)
                            }
                            .contentShape(.rect)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }

            if isLoading {
                Section("Folder page") {
                    ProgressView()
                        .controlSize(.small)
                        .frame(maxWidth: .infinity)
                }
            } else if let page {
                BrowsableResult(start: page, lookup: lookup)
            } else {
                Section("Folder page") {
                    Text("\(folder.prefix) itself was not scraped, or it is hidden by the status or group filter.")
                        .foregroundStyle(.secondary)
                }
            }
        }
        .formStyle(.grouped)
        .textSelection(.enabled)
        // another folder page starts over - browsing keeps the preview running
        .id(page?.targetURL)
    }
}
