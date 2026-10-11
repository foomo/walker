import SwiftUI
import WebKit

/// PagePreview renders a page like a 1280×960 (4:3) desktop window, scaled
/// down to the available width. It is a small browser - onNavigate reports
/// every url the page moves to, links, redirects and history changes alike.
struct PagePreview: View {
    let url: URL
    /// highlight outlines the links to this url and scrolls to the first one
    var highlight: String?
    /// controller lets other views reach into the page, e.g. to highlight a heading
    var controller: PreviewController?
    let onNavigate: (URL) -> Void

    /// the css viewport the page is laid out for
    static let viewport = CGSize(width: 1280, height: 960)

    @State private var state = PreviewState.loading
    /// allowsScripts - JavaScript and cookies of the page, for all previews
    @AppStorage("previewAllowsScripts") private var allowsScripts = true

    enum PreviewState: Equatable {
        case loading
        case loaded
        case failed(String)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            page
            Toggle(isOn: $allowsScripts) {
                Label("JavaScript & cookies", systemImage: "curlybraces")
            }
            .toggleStyle(.switch)
            .controlSize(.mini)
            .font(.caption)
            .foregroundStyle(.secondary)
            .help(allowsScripts ? "Reload the page without JavaScript and cookies" : "Reload the page with JavaScript and cookies")
        }
        .id(url)
    }

    private var page: some View {
        // the empty base takes the offered width and sets the 4:3 size - the web
        // view on top must not influence the layout, it would grow wider
        Color.clear
            .frame(maxWidth: .infinity)
            .aspectRatio(Self.viewport.width / Self.viewport.height, contentMode: .fit)
            .overlay {
                PreviewWebView(url: url, highlight: highlight, allowsScripts: allowsScripts, controller: controller, state: $state, onNavigate: onNavigate)
            }
            .overlay {
                switch state {
                case .loading:
                    ProgressView()
                        .controlSize(.small)
                case .loaded:
                    EmptyView()
                case let .failed(message):
                    ContentUnavailableView("No Preview", systemImage: "eye.slash", description: Text(message))
                        .background(.background)
                }
            }
            .clipShape(.rect(cornerRadius: 6))
            .overlay {
                RoundedRectangle(cornerRadius: 6)
                    .strokeBorder(.separator)
            }
    }
}

private struct PreviewWebView: NSViewRepresentable {
    let url: URL
    let highlight: String?
    let allowsScripts: Bool
    let controller: PreviewController?
    @Binding var state: PagePreview.PreviewState
    let onNavigate: (URL) -> Void

    func makeCoordinator() -> Coordinator {
        Coordinator(state: $state, onNavigate: onNavigate)
    }

    func makeNSView(context: Context) -> ZoomingWebView {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        configuration.mediaTypesRequiringUserActionForPlayback = .all
        configuration.defaultWebpagePreferences.allowsContentJavaScript = allowsScripts
        let webView = ZoomingWebView(frame: .zero, configuration: configuration)
        webView.navigationDelegate = context.coordinator
        webView.uiDelegate = context.coordinator
        webView.allowsBackForwardNavigationGestures = true
        context.coordinator.credential = Self.credential(of: url)
        context.coordinator.highlight = highlight
        context.coordinator.observe(webView)
        controller?.webView = webView
        context.coordinator.allowsScripts = allowsScripts
        let request = URLRequest(url: url)
        Task { @MainActor in
            // the cookie policy must be in place before the first request
            await Coordinator.applyCookiePolicy(allowsScripts, to: webView)
            webView.load(request)
        }
        return webView
    }

    func updateNSView(_ webView: ZoomingWebView, context: Context) {
        context.coordinator.state = $state
        context.coordinator.onNavigate = onNavigate
        context.coordinator.highlight = highlight
        if context.coordinator.allowsScripts != allowsScripts {
            context.coordinator.allowsScripts = allowsScripts
            Task { @MainActor in
                await Coordinator.applyCookiePolicy(allowsScripts, to: webView)
                // the navigation policy applies the javascript setting
                webView.reload()
            }
        }
    }

    /// the web view takes exactly the space it is offered - its own ideal size
    /// must not widen the layout
    func sizeThatFits(_ proposal: ProposedViewSize, nsView: ZoomingWebView, context: Context) -> CGSize? {
        proposal.replacingUnspecifiedDimensions(by: PagePreview.viewport)
    }

    static func dismantleNSView(_ webView: ZoomingWebView, coordinator: Coordinator) {
        coordinator.urlObservation = nil
        webView.stopLoading()
        webView.navigationDelegate = nil
        webView.uiDelegate = nil
    }

    /// credential takes basic auth credentials from the url - walker keeps them
    /// in the target urls of protected sites
    private static func credential(of url: URL) -> URLCredential? {
        guard let user = url.user(percentEncoded: false), !user.isEmpty else { return nil }
        return URLCredential(user: user, password: url.password(percentEncoded: false) ?? "", persistence: .forSession)
    }

    @MainActor
    final class Coordinator: NSObject, WKNavigationDelegate, WKUIDelegate {
        var state: Binding<PagePreview.PreviewState>
        var onNavigate: (URL) -> Void
        var credential: URLCredential?
        var urlObservation: NSKeyValueObservation?
        var highlight: String?
        var allowsScripts = true
        /// once a page was shown, failures leave it on screen
        private var hasLoaded = false

        init(state: Binding<PagePreview.PreviewState>, onNavigate: @escaping (URL) -> Void) {
            self.state = state
            self.onNavigate = onNavigate
        }

        /// observe reports url changes - navigation delegate callbacks miss
        /// history changes of single page apps
        func observe(_ webView: WKWebView) {
            urlObservation = webView.observe(\.url, options: [.new]) { [weak self] webView, _ in
                MainActor.assumeIsolated {
                    if let url = webView.url {
                        self?.onNavigate(url)
                    }
                }
            }
        }

        /// every navigation gets the javascript setting - it can change after the web view was made
        func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, preferences: WKWebpagePreferences) async -> (WKNavigationActionPolicy, WKWebpagePreferences) {
            preferences.allowsContentJavaScript = allowsScripts
            return (.allow, preferences)
        }

        /// applyCookiePolicy accepts cookies, or rejects them and drops the stored ones
        static func applyCookiePolicy(_ allowsCookies: Bool, to webView: WKWebView) async {
            let dataStore = webView.configuration.websiteDataStore
            await dataStore.httpCookieStore.setCookiePolicy(allowsCookies ? .allow : .disallow)
            if !allowsCookies {
                await dataStore.removeData(ofTypes: [WKWebsiteDataTypeCookies], modifiedSince: .distantPast)
            }
        }

        func webView(_ webView: WKWebView, didCommit navigation: WKNavigation!) {
            // a new document - make sure it gets the zoom
            (webView as? ZoomingWebView)?.applyZoom(force: true)
        }

        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            hasLoaded = true
            state.wrappedValue = .loaded
            if let highlight, let target = try? String(data: JSONEncoder().encode(highlight), encoding: .utf8) {
                webView.evaluateJavaScript(Self.highlightScript + "(\(target))")
            }
        }

        /// highlightScript outlines every link to the target - compared without
        /// fragment and credentials - and scrolls the first one into view
        private static let highlightScript = """
        (function(target) {
            const norm = (u) => {
                try {
                    const url = new URL(u, location.href)
                    url.hash = ''; url.username = ''; url.password = ''
                    return url.href
                } catch (e) { return u }
            }
            const wanted = norm(target)
            let first = null
            document.querySelectorAll('a[href]').forEach((a) => {
                if (norm(a.href) !== wanted) return
                a.style.outline = '3px solid #ff3b30'
                a.style.outlineOffset = '2px'
                a.style.borderRadius = '3px'
                first = first || a
            })
            if (first) first.scrollIntoView({ block: 'center' })
        })
        """

        func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
            fail(error)
        }

        func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
            fail(error)
        }

        private func fail(_ error: Error) {
            if hasLoaded || (error as NSError).code == NSURLErrorCancelled { return }
            state.wrappedValue = .failed(error.localizedDescription)
        }

        func webView(_ webView: WKWebView, respondTo challenge: URLAuthenticationChallenge) async -> (URLSession.AuthChallengeDisposition, URLCredential?) {
            let method = challenge.protectionSpace.authenticationMethod
            if let credential, challenge.previousFailureCount == 0,
               method == NSURLAuthenticationMethodHTTPBasic || method == NSURLAuthenticationMethodHTTPDigest {
                return (.useCredential, credential)
            }
            return (.performDefaultHandling, nil)
        }

        /// target="_blank" links and window.open load in the preview itself
        func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
            if navigationAction.targetFrame == nil {
                webView.load(navigationAction.request)
            }
            return nil
        }
    }
}

/// ZoomingWebView scales the page with pageZoom, so that the viewport is
/// PagePreview.viewport wide whatever width the view has - taken from its
/// actual bounds, so it always matches what is on screen.
final class ZoomingWebView: WKWebView {
    override func setFrameSize(_ newSize: NSSize) {
        super.setFrameSize(newSize)
        applyZoom()
    }

    func applyZoom(force: Bool = false) {
        guard bounds.width > 0 else { return }
        let zoom = bounds.width / PagePreview.viewport.width
        if force || abs(pageZoom - zoom) > 0.001 {
            pageZoom = zoom
        }
    }
}

/// PreviewController reaches into the page of a preview - it highlights a
/// heading of the page, as walker extracted it.
@MainActor
final class PreviewController {
    weak var webView: WKWebView?

    /// highlightHeading outlines the heading and scrolls it into view - nil
    /// removes the highlight
    func highlightHeading(_ heading: HeadingOutline.Position?) {
        guard let webView else { return }
        let arguments: String
        if let heading,
           let text = try? String(data: JSONEncoder().encode(heading.text), encoding: .utf8) {
            arguments = "\(heading.level), \(text), \(heading.occurrence), \(heading.levelIndex)"
        } else {
            arguments = "0, '', 0, 0"
        }
        webView.evaluateJavaScript(Self.script + "(\(arguments))")
    }

    /// script finds the heading by level and text - whitespace collapsed, the
    /// live page may differ from the scraped html - and falls back to the
    /// n-th heading of the level
    private static let script = """
    (function(level, text, occurrence, levelIndex) {
        const css = 'walker-heading-highlight'
        if (!document.getElementById(css)) {
            const style = document.createElement('style')
            style.id = css
            style.textContent = '.' + css + ' { outline: 3px solid #0a84ff !important; outline-offset: 4px !important; background: rgba(10, 132, 255, 0.15) !important; border-radius: 3px; }'
            document.head.appendChild(style)
        }
        document.querySelectorAll('.' + css).forEach((el) => el.classList.remove(css))
        if (!level) return
        const norm = (s) => (s || '').replace(/\\s+/g, ' ').trim()
        const wanted = norm(text)
        const candidates = Array.from(document.querySelectorAll('h' + level))
        const matches = candidates.filter((el) => norm(el.textContent) === wanted)
        const el = matches[occurrence] || candidates[levelIndex]
        if (!el) return
        el.classList.add(css)
        el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    })
    """
}
