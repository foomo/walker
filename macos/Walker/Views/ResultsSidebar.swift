import SwiftUI

/// ResultsSidebar filters the results by status, group and folder. Selecting a
/// folder sets the prefix, status and group rows toggle their filter - all of
/// them combine.
struct ResultsSidebar: View {
    let folders: [WalkerVO.Folder]
    let statusOptions: [Walker.StatusStats]
    let groupOptions: [Walker.GroupStats]
    let bucketOptions: [WalkerVO.PerformanceBucket]
    @Binding var prefix: String
    @Binding var status: Int?
    @Binding var group: String?
    @Binding var duration: ServerModel.DurationRange?

    @State private var showsStatus = true
    @State private var showsPerformance = true
    @State private var showsGroups = true
    @State private var showsFolders = true

    var body: some View {
        List(selection: $prefix) {
            if !statusOptions.isEmpty {
                Section("Status", isExpanded: $showsStatus) {
                    ForEach(statusOptions, id: \.code) { option in
                        FilterRow(
                            title: StatusClass.label(for: option.code),
                            count: option.count,
                            color: StatusClass.color(for: option.code),
                            isActive: status == option.code
                        ) {
                            status = status == option.code ? nil : option.code
                        }
                    }
                }
            }
            if bucketOptions.contains(where: { $0.pages > 0 }) {
                Section("Performance", isExpanded: $showsPerformance) {
                    ForEach(bucketOptions.shares()) { share in
                        let range = ServerModel.DurationRange(bucketOptions[share.index])
                        if share.pages > 0 || duration == range {
                            FilterRow(
                                title: share.label,
                                count: share.pages,
                                color: share.color,
                                isActive: duration == range
                            ) {
                                duration = duration == range ? nil : range
                            }
                            .help(share.name)
                        }
                    }
                }
            }
            // without a group header every result is in the same empty group
            if groupOptions.count > 1 || group != nil {
                Section("Groups", isExpanded: $showsGroups) {
                    ForEach(groupOptions, id: \.group) { option in
                        FilterRow(
                            title: option.group.isEmpty ? "No group" : option.group,
                            count: option.count,
                            color: .accentColor,
                            isActive: group == option.group
                        ) {
                            group = group == option.group ? nil : option.group
                        }
                    }
                }
            }
            Section("Folders", isExpanded: $showsFolders) {
                OutlineGroup(folders, id: \.prefix, children: \.folders) { folder in
                    FolderLabel(folder: folder, isHost: folders.contains { $0.prefix == folder.prefix })
                        .tag(folder.prefix)
                }
            }
        }
        .listStyle(.sidebar)
        .overlay {
            if folders.isEmpty && statusOptions.isEmpty {
                ContentUnavailableView("No Results", systemImage: "tray", description: Text("There are no results to filter yet."))
            }
        }
    }
}

/// FilterRow toggles a filter - it is not part of the list selection, which
/// belongs to the folders.
struct FilterRow: View {
    let title: String
    let count: Int
    let color: Color
    let isActive: Bool
    let toggle: () -> Void

    var body: some View {
        Button(action: toggle) {
            HStack {
                Label {
                    Text(title)
                        .fontWeight(isActive ? .semibold : .regular)
                } icon: {
                    Image(systemName: isActive ? "checkmark.circle.fill" : "circle.fill")
                        .foregroundStyle(color)
                        .imageScale(isActive ? .medium : .small)
                }
                Spacer(minLength: 4)
            }
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .badge(count)
        .help(isActive ? "Remove this filter" : "Only show \(title)")
    }
}

struct FolderLabel: View {
    let folder: WalkerVO.Folder
    let isHost: Bool

    var body: some View {
        HStack(spacing: 6) {
            Label(folder.name, systemImage: isHost ? "globe" : "folder")
                .lineLimit(1)
                .truncationMode(.middle)
            Spacer(minLength: 4)
            if folder.errors > 0 {
                Label(String(folder.errors), systemImage: "exclamationmark.triangle.fill")
                    .labelStyle(.titleAndIcon)
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.orange)
                    .help("\(folder.errors) pages with errors")
            }
        }
        .badge(folder.pages)
        .help(folder.prefix)
    }
}
