import SwiftUI

/// Crumb is one level of a folder like "https://example.com/shop/men".
struct Crumb: Identifiable, Equatable {
    let title: String
    let prefix: String
    let isHost: Bool

    var id: String { prefix }

    /// crumbs splits a folder prefix - as in Folder.prefix, without a
    /// trailing slash - into its levels, starting with the host
    static func crumbs(for prefix: String) -> [Crumb] {
        guard let schemeEnd = prefix.range(of: "://") else { return [] }
        let rest = prefix[schemeEnd.upperBound...]
        let pathStart = rest.firstIndex(of: "/") ?? rest.endIndex
        var host = String(rest[..<pathStart])
        if let at = host.lastIndex(of: "@") {
            host = String(host[host.index(after: at)...])
        }
        guard !host.isEmpty else { return [] }
        var current = String(prefix[..<pathStart])
        var crumbs = [Crumb(title: host, prefix: current, isHost: true)]
        for segment in rest[pathStart...].split(separator: "/") {
            current += "/" + segment
            crumbs.append(Crumb(title: String(segment), prefix: current, isHost: false))
        }
        return crumbs
    }
}

/// FilterChip is an active status or group filter, shown in the breadcrumb bar
/// so that it stays visible and removable while the sidebar is hidden.
struct FilterChip: Identifiable {
    let id: String
    let title: String
    let color: Color
    let remove: () -> Void
}

/// Breadcrumbs shows the selected folder - every level but the last navigates
/// up - and the active filters.
struct Breadcrumbs: View {
    let crumbs: [Crumb]
    let folder: WalkerVO.Folder?
    let chips: [FilterChip]
    @Binding var prefix: String

    private var host: Crumb? { crumbs.first }
    /// path are the folder levels below the host
    private var path: ArraySlice<Crumb> { crumbs.dropFirst() }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            // first line: the server, the filters and the counts
            HStack(spacing: 8) {
                HStack(spacing: 4) {
                    crumbButton("All Pages", systemImage: "tray.full", prefix: "")
                    if let host {
                        separator
                        crumb(host)
                    }
                }
                .lineLimit(1)
                .truncationMode(.middle)
                .layoutPriority(1)

                ForEach(chips) { chip in
                    Button(action: chip.remove) {
                        HStack(spacing: 4) {
                            Circle()
                                .fill(chip.color)
                                .frame(width: 7, height: 7)
                            Text(chip.title)
                            Image(systemName: "xmark")
                                .font(.caption2.weight(.bold))
                                .foregroundStyle(.secondary)
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 2)
                        .background(.quaternary, in: .capsule)
                        .contentShape(.capsule)
                    }
                    .buttonStyle(.plain)
                    .help("Remove the filter \(chip.title)")
                }

                Spacer(minLength: 8)

                if let folder {
                    Text("\(folder.pages) pages")
                        .monospacedDigit()
                        .foregroundStyle(.secondary)
                    if folder.errors > 0 {
                        Label(String(folder.errors), systemImage: "exclamationmark.triangle.fill")
                            .monospacedDigit()
                            .foregroundStyle(.orange)
                            .help("\(folder.errors) pages with errors")
                    }
                }
            }

            // second line: the path below the server
            if !path.isEmpty {
                ScrollView(.horizontal) {
                    HStack(spacing: 4) {
                        ForEach(path) { crumb in
                            if crumb != path.first {
                                separator
                            }
                            self.crumb(crumb)
                        }
                    }
                    .lineLimit(1)
                    .padding(.vertical, 1)
                }
                .scrollIndicators(.hidden)
                // short paths sit left, long ones show their end - the current folder
                .defaultScrollAnchor(.leading, for: .alignment)
                .defaultScrollAnchor(.trailing, for: .initialOffset)
                .defaultScrollAnchor(.trailing, for: .sizeChanges)
            }
        }
        .font(.callout)
        .padding(.horizontal)
        .padding(.vertical, 6)
    }

    private var separator: some View {
        Image(systemName: "chevron.compact.right")
            .foregroundStyle(.tertiary)
    }

    /// crumb - the current folder is bold, every level above navigates up
    @ViewBuilder
    private func crumb(_ crumb: Crumb) -> some View {
        if crumb == crumbs.last {
            Label(crumb.title, systemImage: crumb.isHost ? "globe" : "folder.fill")
                .fontWeight(.semibold)
        } else {
            crumbButton(crumb.title, systemImage: crumb.isHost ? "globe" : "folder", prefix: crumb.prefix)
        }
    }

    private func crumbButton(_ title: String, systemImage: String, prefix target: String) -> some View {
        Button {
            prefix = target
        } label: {
            Label(title, systemImage: systemImage)
        }
        .buttonStyle(.plain)
        .foregroundStyle(.secondary)
        .help(target.isEmpty ? "Show all pages" : target)
    }
}

extension Array where Element == WalkerVO.Folder {
    /// folder finds the folder with the prefix anywhere in the tree
    func folder(withPrefix prefix: String) -> WalkerVO.Folder? {
        for folder in self {
            if folder.prefix == prefix {
                return folder
            }
            if prefix.isInFolder(folder.prefix), let found = (folder.folders ?? []).folder(withPrefix: prefix) {
                return found
            }
        }
        return nil
    }
}
