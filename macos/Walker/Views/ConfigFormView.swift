import AppKit
import SwiftUI

/// ConfigFormView edits a walker yaml config as a form. The yaml stays the
/// truth: every edit is written into it by the configvalidator helper -
/// comments and unknown keys survive - and validated on the way. Each field
/// explains what it does to the crawl.
///
/// The config editor window edits its document with it, the detail of a
/// local config the file on disk. Header and footer are sections of the
/// container around the config.
struct ConfigFormView<Header: View, Footer: View>: View {
    @Binding var text: String
    /// base is the directory relative paths of the config are resolved against
    let base: String
    @Binding var problems: [ConfigTool.Problem]
    @ViewBuilder var header: Header
    @ViewBuilder var footer: Footer

    @State private var form = ConfigTool.Form()
    /// readForm is the form as the yaml has it - only edits beyond it are written,
    /// so opening a file never changes it
    @State private var readForm: ConfigTool.Form?
    @State private var error: String?
    @State private var isLoaded = false
    /// written is the text of the last own write - it changes the text, but must not reset the form
    @State private var written: String?
    /// edits counts form changes, the debounced write waits for the last one
    @State private var edits = 0

    var body: some View {
        Form {
            header
            if let error {
                Section {
                    Label(error, systemImage: "xmark.octagon")
                        .foregroundStyle(.red)
                }
            }
            Section("Target") {
                TextField("Base URL", text: $form.baseURL, prompt: Text("https://www.example.com"))
                FieldNote("Where the crawl starts. Only links to this scheme and host are followed: `https://shop.example` skips `http://shop.example` and `https://blog.shop.example`. With no paths, the path of the URL is the start path - `https://shop.example/de/` crawls `/de/…` only.")
                ProblemList(problems: problems(for: "target"))
                StringListEditor(title: "Paths", placeholder: "/shop", items: $form.paths)
                FieldNote("Start paths - and the only path prefixes that are followed. With `/de` and `/fr`, walker starts on both and never visits `/en/…`. Empty: `/`, the whole site.")
            }
            Section("Crawling") {
                Stepper(value: $form.concurrency, in: 1...50) {
                    LabeledContent("Concurrency", value: "\(form.concurrency)")
                }
                FieldNote("Requests in parallel, each with its own HTTP client. Walker does not throttle beyond that: 2 is gentle, above 20 a site can go down.")
                ProblemList(problems: problems(for: "concurrency"))
                Stepper(value: $form.depth, in: 0...50) {
                    LabeledContent("Depth", value: form.depth == 0 ? "unlimited" : "\(form.depth)")
                }
                FieldNote("The most path segments a followed link may have. With 2, `/shop/shoes` is crawled, `/shop/shoes/sneaker` and `/shop/shoes/` are not - a trailing slash counts as a segment.")
                ProblemList(problems: problems(for: "depth"))
                Toggle("Follow paging", isOn: $form.paging)
                FieldNote("Follow the `<link rel=\"next\">` and `rel=\"prev\"` of a page. Off: page 2, 3, … of a list are only crawled when an ordinary link points to them.")
                Toggle("Use cookies", isOn: $form.useCookies)
                FieldNote("Every client keeps the cookies it gets, like a browser session: a consent or session cookie from the first page is sent with the next ones. Off: every request is a first visit.")
                Toggle("Ignore robots.txt", isOn: $form.ignoreRobots)
                FieldNote("Off: the robots.txt of the host is read for the user agent - disallowed links are skipped, and a disallowed start path stops the crawl. On: everything is crawled.")
                TextField("User agent", text: $form.agent)
                FieldNote("Sent with every request, and the group of robots.txt walker follows: `Disallow` under `User-agent: foomo-walker` applies to the default.")
                ProblemList(problems: problems(for: "agent"))
            }
            Section("Filters") {
                StringListEditor(title: "Ignore path prefixes", placeholder: "/admin", items: $form.ignore)
                FieldNote("Links whose path starts with one of these are skipped. It is a plain prefix: `/admin` skips `/admin/users` and `/administration` too - `/admin/` keeps the latter.")
                ProblemList(problems: problems(for: "ignore"))
                StringListEditor(title: "Ignore queries with", placeholder: "sort", items: $form.ignoreQueriesWith)
                FieldNote("Links with one of these query parameters are skipped: `sort` skips `/shoes?sort=price` and keeps `/shoes?page=2`. Against endless filter combinations.")
                Toggle("Ignore all queries", isOn: $form.ignoreAllQueries)
                FieldNote("Skips every link with a query: `/shoes?page=2` is not crawled, `/shoes` is.")
            }
            Section("Schedule") {
                TextField("Interval", text: $form.interval, prompt: Text("e.g. 1h - empty restarts right away"))
                FieldNote("Crawls start on wall clock boundaries: `1h` at every full hour, `30m` at :00 and :30, `24h` at midnight. Empty: the next crawl starts as soon as one is done. A crawl that takes longer than the interval is followed right away, then the schedule picks up again.")
                ProblemList(problems: problems(for: "interval"))
                TextField("Offset", text: $form.offset, prompt: Text("e.g. 15m"))
                FieldNote("Shifts the boundaries: `24h` with `3h` crawls at 03:00 at night, `1h` with `15m` at a quarter past every hour.")
                ProblemList(problems: problems(for: "offset"))
            }
            Section("HTML Schema") {
                LabeledContent("Schema root") {
                    HStack {
                        TextField("Schema root", text: $form.schemaRoot, prompt: Text("relative to this file"))
                            .labelsHidden()
                        Button("Choose…", action: chooseSchemaRoot)
                    }
                }
                FieldNote("A directory of semantic shapes, `*.shape.html`: `default.shape.html` and `groups/`. Every page is validated against the shape of its group, and gets findings, a score and a penalty. Empty: no validation.")
                ProblemList(problems: problems(for: "schemaroot"))
                TextField("Group header", text: $form.groupHeader, prompt: Text("x-next-js-app"))
                FieldNote("The response header that names the group of a page: `x-next-js-app: product` validates with `groups/product.shape.html`. Pages without it are validated with `default.shape.html`.")
                ProblemList(problems: problems(for: "groupheader"))
            }
            Section("Server") {
                TextField("Address", text: $form.addr, prompt: Text(":3001"))
                FieldNote("Where walker serves its reports, metrics and this app: `:3001` on all interfaces, `127.0.0.1:3001` only on this Mac. Two walkers can not share a port.")
                ProblemList(problems: problems(for: "addr"))
            }
            Section("Problems") {
                if problems.isEmpty {
                    Label(isLoaded ? "No problems" : "Validating…", systemImage: "checkmark.seal")
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(problems, id: \.self) { problem in
                        ProblemRow(problem: problem, showsLine: true)
                    }
                }
            }
            footer
        }
        .formStyle(.grouped)
        .disabled(!isLoaded && error == nil)
        // text from outside - the file, undo - is read into the form
        .task(id: text) {
            await read(text)
        }
        .onChange(of: form) {
            guard isLoaded, form != readForm else { return }
            edits += 1
        }
        .task(id: edits) {
            guard edits > 0 else { return }
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            await write()
        }
    }

    private func problems(for key: String) -> [ConfigTool.Problem] {
        problems.filter { $0.key == key }
    }

    private func read(_ text: String) async {
        do {
            let result = try await ConfigTool.read(yaml: text, base: base)
            guard !Task.isCancelled else { return }
            problems = result.problems
            // after an own write the form already is what was written
            if text != written {
                readForm = result.form
                form = result.form
                isLoaded = true
            }
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func write() async {
        guard form != readForm else { return }
        let edited = form
        do {
            let new = try await ConfigTool.write(yaml: text, form: edited)
            readForm = edited
            guard new != text else { return }
            written = new
            text = new
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func chooseSchemaRoot() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.message = "Choose the schema root - the directory with default.shape.html and groups/"
        if !base.isEmpty {
            panel.directoryURL = URL(filePath: base)
        }
        guard panel.runModal() == .OK, let url = panel.url else { return }
        let path = url.path(percentEncoded: false)
        // inside the config's directory the path is written relative, like walker resolves it
        if !base.isEmpty, path.hasPrefix(base + "/") {
            form.schemaRoot = String(path.dropFirst(base.count + 1))
        } else {
            form.schemaRoot = path
        }
    }
}

extension ConfigTool.Problem {
    static func summary(_ problems: [ConfigTool.Problem]) -> String {
        let errors = problems.filter { $0.severity == "error" }.count
        let warnings = problems.filter { $0.severity == "warning" }.count
        return errors + warnings == 0 ? "valid" : "\(errors) errors, \(warnings) warnings"
    }
}

/// FieldNote explains a field: what it does to the crawl, with examples - `code` is markdown
private struct FieldNote: View {
    let text: LocalizedStringKey

    init(_ text: LocalizedStringKey) {
        self.text = text
    }

    var body: some View {
        Text(text)
            .font(.caption)
            .foregroundStyle(.secondary)
            .fixedSize(horizontal: false, vertical: true)
    }
}

/// StringListEditor edits a list of strings - one text field per entry
private struct StringListEditor: View {
    let title: String
    let placeholder: String
    @Binding var items: [String]

    // the entries below the title, left aligned - a list of paths reads
    // badly right aligned in the value column of the grouped form
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title)
            ForEach(items.indices, id: \.self) { index in
                HStack {
                    TextField(title, text: Binding(
                        get: { index < items.count ? items[index] : "" },
                        set: { if index < items.count { items[index] = $0 } }
                    ), prompt: Text(placeholder))
                    .labelsHidden()
                    .multilineTextAlignment(.leading)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    Button {
                        items.remove(at: index)
                    } label: {
                        Image(systemName: "minus.circle")
                    }
                    .buttonStyle(.borderless)
                    .help("Remove")
                }
            }
            Button {
                items.append("")
            } label: {
                Label("Add", systemImage: "plus.circle")
            }
            .buttonStyle(.borderless)
        }
    }
}

/// ProblemList shows the problems of one field below it
private struct ProblemList: View {
    let problems: [ConfigTool.Problem]

    var body: some View {
        ForEach(problems, id: \.self) { problem in
            ProblemRow(problem: problem, showsLine: false)
        }
    }
}

struct ProblemRow: View {
    let problem: ConfigTool.Problem
    let showsLine: Bool

    var body: some View {
        Label {
            VStack(alignment: .leading, spacing: 1) {
                Text(problem.message)
                if showsLine {
                    Text("line \(problem.line)\(problem.key.isEmpty ? "" : " · \(problem.key)")")
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                }
            }
        } icon: {
            Image(systemName: icon)
                .foregroundStyle(color)
        }
        .font(.callout)
    }

    private var icon: String {
        switch problem.severity {
        case "error": "xmark.octagon.fill"
        case "warning": "exclamationmark.triangle.fill"
        default: "info.circle.fill"
        }
    }

    private var color: Color {
        switch problem.severity {
        case "error": .red
        case "warning": .orange
        default: .blue
        }
    }
}
