import SwiftUI

/// OpenGraph is the OpenGraph data of a validated page, read from the meta
/// elements of its head - every value keeps its locator, to link it with the
/// schema tree
struct OpenGraph {
    struct Value {
        let content: String
        let locator: String
    }

    private(set) var values: [String: Value] = [:]

    /// the basics every link preview needs
    static let basics = ["og:title", "og:description", "og:image", "og:url"]

    /// init reads og:*, product:* and twitter:* meta elements - nil if the page has no og:* at all
    init?(page: HTMLSchemaVO.PageData) {
        for element in (page.head ?? []).compactMap({ $0 }) where element.tag == "meta" {
            let attributes = element.attributes ?? []
            func value(_ key: String) -> String? {
                attributes.first { $0.key == key }?.value
            }
            guard let key = value("property") ?? value("name"),
                  key.hasPrefix("og:") || key.hasPrefix("product:") || key.hasPrefix("twitter:"),
                  values[key] == nil else { continue }
            values[key] = Value(content: value("content") ?? "", locator: element.locator)
        }
        guard values.keys.contains(where: { $0.hasPrefix("og:") }) else { return nil }
    }

    subscript(_ key: String) -> Value? {
        values[key]
    }

    /// previewKeys are the meta elements the link preview is made of
    static let previewKeys = [
        "og:image", "og:url", "og:site_name", "og:title", "og:description", "og:type",
        "product:price:amount", "product:price:currency",
    ]

    /// previewLocators are the head elements the link preview shows
    var previewLocators: Set<String> {
        Set(Self.previewKeys.compactMap { values[$0]?.locator })
    }

    /// isPreviewSource tells whether a missing element - as the schema writes it -
    /// is one the link preview is made of
    static func isPreviewSource(_ source: String) -> Bool {
        previewKeys.contains { source.contains("property=\"\($0)\"") }
    }

    var missingBasics: [String] {
        Self.basics.filter { (values[$0]?.content ?? "").isEmpty }
    }
}

/// OpenGraphCard renders the OpenGraph data like a link preview in a social
/// network. Its parts are linked with the schema tree: a click selects the
/// schema node of the meta element, a selected node outlines its part.
struct OpenGraphCard: View {
    let openGraph: OpenGraph
    let model: SchemaDevModel

    private var highlight: Color {
        model.headMark?.color ?? .accentColor
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            card
            ForEach(openGraph.missingBasics, id: \.self) { key in
                Label("\(key) is missing - networks fall back to guesses", systemImage: "exclamationmark.triangle")
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
        }
        .padding(.vertical, 4)
    }

    private var card: some View {
        VStack(alignment: .leading, spacing: 0) {
            image
                .part("og:image", in: self)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 4) {
                    Text(domain.uppercased())
                        .part("og:url", in: self)
                    if let site = openGraph["og:site_name"], !site.content.isEmpty {
                        Text("·")
                        Text(site.content)
                            .part("og:site_name", in: self)
                    }
                }
                .font(.caption2)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                Text(openGraph["og:title"]?.content ?? "No og:title")
                    .font(.headline)
                    .lineLimit(2)
                    .foregroundStyle(openGraph["og:title"] == nil ? Color.secondary : Color.primary)
                    .part("og:title", in: self)
                Text(openGraph["og:description"]?.content ?? "No og:description")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                    .part("og:description", in: self)
                if let amount = openGraph["product:price:amount"] {
                    HStack(spacing: 4) {
                        Text(amount.content)
                            .part("product:price:amount", in: self)
                        Text(openGraph["product:price:currency"]?.content ?? "")
                            .part("product:price:currency", in: self)
                    }
                    .font(.caption.bold())
                }
                if let type = openGraph["og:type"] {
                    Text(type.content)
                        .font(.caption2.monospaced())
                        .padding(.horizontal, 5)
                        .padding(.vertical, 1)
                        .background(Capsule().fill(.quaternary))
                        .part("og:type", in: self)
                }
            }
            .padding(10)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .background(.background)
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .overlay {
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(.separator)
        }
    }

    @ViewBuilder
    private var image: some View {
        if let value = openGraph["og:image"], let url = URL(string: value.content), !value.content.isEmpty {
            if url.pathExtension.lowercased() == "svg" {
                placeholder("SVG - social networks show no preview", systemImage: "photo.badge.exclamationmark", color: .orange)
            } else {
                AsyncImage(url: url) { phase in
                    switch phase {
                    case let .success(image):
                        image
                            .resizable()
                            .aspectRatio(contentMode: .fill)
                    case .failure:
                        placeholder("og:image could not be loaded", systemImage: "photo.badge.exclamationmark", color: .red)
                    default:
                        ProgressView()
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                    }
                }
                .frame(height: 170)
                .frame(maxWidth: .infinity)
                .clipped()
                .help(value.content)
            }
        } else {
            placeholder("No og:image", systemImage: "photo", color: .secondary)
        }
    }

    private func placeholder(_ text: String, systemImage: String, color: Color) -> some View {
        Label(text, systemImage: systemImage)
            .font(.caption)
            .foregroundStyle(color)
            .frame(maxWidth: .infinity, minHeight: 90)
            .background(.quaternary.opacity(0.5))
    }

    /// domain is the host of og:url, as networks show it
    private var domain: String {
        guard let url = openGraph["og:url"].flatMap({ URL(string: $0.content) }), let host = url.host() else {
            return "no og:url"
        }
        return host
    }

    fileprivate func isSelected(_ key: String) -> Bool {
        guard let locator = openGraph[key]?.locator else { return false }
        return model.selectedLocators.contains(locator)
    }

    fileprivate func select(_ key: String) {
        if let locator = openGraph[key]?.locator {
            model.select(locator: locator)
        }
    }

    fileprivate var highlightColor: Color {
        highlight
    }
}

private extension View {
    /// part links a part of the card with the meta element of key
    func part(_ key: String, in card: OpenGraphCard) -> some View {
        let selected = card.isSelected(key)
        return self
            .overlay {
                if selected {
                    RoundedRectangle(cornerRadius: 4)
                        .strokeBorder(card.highlightColor, lineWidth: 2)
                        .padding(-2)
                }
            }
            .contentShape(.rect)
            .onTapGesture { card.select(key) }
            .help(key)
    }
}
