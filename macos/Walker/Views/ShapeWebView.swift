import SwiftUI
import WebKit

/// ShapeWebView is the browser of the shape developer - a full size web
/// view, onLoad reports every page that finished loading
struct ShapeWebView: NSViewRepresentable {
    let url: URL?
    /// reloadCount reloads the page whenever it changes
    let reloadCount: Int
    let highlighter: ShapeHighlighter
    let onLoad: (URL) -> Void
    let onFail: (String) -> Void

    func makeCoordinator() -> Coordinator {
        Coordinator(onLoad: onLoad, onFail: onFail)
    }

    func makeNSView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.mediaTypesRequiringUserActionForPlayback = .all
        let webView = WKWebView(frame: .zero, configuration: configuration)
        webView.navigationDelegate = context.coordinator
        webView.uiDelegate = context.coordinator
        webView.allowsBackForwardNavigationGestures = true
        // a developer tool - the web inspector is welcome
        webView.isInspectable = true
        context.coordinator.observe(webView)
        highlighter.webView = webView
        load(in: webView, coordinator: context.coordinator)
        return webView
    }

    func updateNSView(_ webView: WKWebView, context: Context) {
        let coordinator = context.coordinator
        coordinator.onLoad = onLoad
        coordinator.onFail = onFail
        highlighter.webView = webView
        if url != coordinator.requestedURL {
            load(in: webView, coordinator: coordinator)
        } else if reloadCount != coordinator.reloadCount {
            coordinator.reloadCount = reloadCount
            webView.reload()
        }
    }

    private func load(in webView: WKWebView, coordinator: Coordinator) {
        coordinator.requestedURL = url
        coordinator.reloadCount = reloadCount
        guard let url else { return }
        coordinator.credential = Self.credential(of: url)
        webView.load(URLRequest(url: url))
    }

    static func dismantleNSView(_ webView: WKWebView, coordinator: Coordinator) {
        coordinator.urlObservation = nil
        coordinator.pending?.cancel()
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
        var onLoad: (URL) -> Void
        var onFail: (String) -> Void
        var requestedURL: URL?
        var reloadCount = 0
        var credential: URLCredential?
        var urlObservation: NSKeyValueObservation?
        /// pending is the debounced report of a navigation without a load
        var pending: Task<Void, Never>?
        /// loadedURL is the url last reported, without fragment
        private var loadedURL: URL?

        /// settle is the time a single page app gets to render a new route
        private static let settle = Swift.Duration.milliseconds(700)

        init(onLoad: @escaping (URL) -> Void, onFail: @escaping (String) -> Void) {
            self.onLoad = onLoad
            self.onFail = onFail
        }

        /// observe reports url changes, that load no document - history changes
        /// of single page apps and the back forward cache. Loads report in didFinish.
        func observe(_ webView: WKWebView) {
            urlObservation = webView.observe(\.url, options: [.new]) { [weak self] webView, _ in
                MainActor.assumeIsolated {
                    self?.urlChanged(webView)
                }
            }
        }

        private func urlChanged(_ webView: WKWebView) {
            guard let url = webView.url, !webView.isLoading, Self.document(url) != loadedURL else { return }
            pending?.cancel()
            pending = Task { [weak self, weak webView] in
                try? await Task.sleep(for: Self.settle)
                guard !Task.isCancelled, let self, let webView, !webView.isLoading, let url = webView.url else { return }
                self.report(url)
            }
        }

        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            pending?.cancel()
            if let url = webView.url {
                report(url)
            }
        }

        private func report(_ url: URL) {
            loadedURL = Self.document(url)
            onLoad(url)
        }

        /// document is the url without fragment - a fragment change stays in the same document
        private static func document(_ url: URL) -> URL? {
            var components = URLComponents(url: url, resolvingAgainstBaseURL: false)
            components?.fragment = nil
            return components?.url
        }

        func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
            fail(error)
        }

        func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
            fail(error)
        }

        private func fail(_ error: Error) {
            if (error as NSError).code == NSURLErrorCancelled { return }
            onFail(error.localizedDescription)
        }

        func webView(_ webView: WKWebView, respondTo challenge: URLAuthenticationChallenge) async -> (URLSession.AuthChallengeDisposition, URLCredential?) {
            let method = challenge.protectionSpace.authenticationMethod
            if let credential, challenge.previousFailureCount == 0,
               method == NSURLAuthenticationMethodHTTPBasic || method == NSURLAuthenticationMethodHTTPDigest {
                return (.useCredential, credential)
            }
            return (.performDefaultHandling, nil)
        }

        /// target="_blank" links and window.open load in the web view itself
        func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
            if navigationAction.targetFrame == nil {
                webView.load(navigationAction.request)
            }
            return nil
        }
    }
}

/// ShapeHighlighter reaches into the page of the shape developer: it
/// outlines the page nodes of a trace node and takes DOM snapshots
@MainActor
final class ShapeHighlighter {
    weak var webView: WKWebView?

    /// pageURL is the http(s) url of the page in the web view - nil for about:blank and while there is none
    var pageURL: URL? {
        guard let url = webView?.url, url.scheme == "http" || url.scheme == "https" else { return nil }
        return url
    }

    /// isPageLoaded is true when a web page finished loading
    var isPageLoaded: Bool {
        guard let webView else { return false }
        return pageURL != nil && !webView.isLoading
    }

    /// Resolve the configured group header using the browser's current session.
    func groupHeaderValue(_ header: String) async throws -> String {
        guard let webView else { return "" }
        return try await webView.callAsyncJavaScript(
            Self.groupHeaderScript, arguments: ["header": header], in: nil, contentWorld: .page
        ) as? String ?? ""
    }

    // Fetch in the browser so cookies and client-side navigation use the current page.
    private static let groupHeaderScript = """
    const url = window.location.href;
    const options = {cache: 'no-store', credentials: 'same-origin', signal: AbortSignal.timeout(10000)};
    let response = await fetch(url, {...options, method: 'HEAD'});
    if (response.status === 405 || response.status === 501) {
        response = await fetch(url, {...options, method: 'GET'});
        if (response.body) await response.body.cancel();
    }
    if (!response.ok) throw new Error('Could not read page group: HTTP ' + response.status);
    const group = response.headers.get(header) || '';
    return group.endsWith('/') ? group + 'index' : group;
    """

    /// snapshot is the html of the DOM as it is now
    func snapshot() async -> String? {
        guard let webView else { return nil }
        return await withCheckedContinuation { continuation in
            webView.evaluateJavaScript("document.documentElement.outerHTML") { result, _ in
                continuation.resume(returning: result as? String)
            }
        }
    }

    /// highlight outlines the locators of the node in its status color - dashed
    /// where something is missing - and scrolls to the first one. It returns
    /// how many locators were found in the page, nil removes the highlight.
    @discardableResult
    func highlight(_ node: SemanticShapeVO.TraceNode?, scroll: Bool = true) async -> (found: Int, total: Int) {
        await highlight(
            locators: node?.locators ?? [],
            color: node.map { $0.status.cssColor } ?? "",
            missing: node.map { !$0.found } ?? false,
            scroll: scroll
        )
    }

    /// highlight outlines the elements at the locators in a css color
    @discardableResult
    func highlight(locators: [String], color: String, missing: Bool = false, scroll: Bool) async -> (found: Int, total: Int) {
        guard let webView else { return (0, 0) }
        let encoder = JSONEncoder()
        guard let locatorsJSON = try? String(data: encoder.encode(locators), encoding: .utf8) else { return (0, 0) }
        let script = Self.script + "(\(locatorsJSON), '\(color)', \(missing), \(scroll))"
        let found = await withCheckedContinuation { continuation in
            webView.evaluateJavaScript(script) { result, _ in
                continuation.resume(returning: (result as? NSNumber)?.intValue ?? 0)
            }
        }
        return (found, locators.count)
    }

    /// script marks the elements with a class - a style element in the page
    /// draws the outline, the color comes from a css variable on the element
    private static let script = """
    (function(locators, color, missing, scroll) {
        const css = 'walker-shape-highlight'
        if (!document.getElementById(css)) {
            const style = document.createElement('style')
            style.id = css
            style.textContent = '.' + css + ' { outline: 3px var(--walker-outline-style, solid) var(--walker-color) !important; outline-offset: 2px !important; background-color: color-mix(in srgb, var(--walker-color) 12%, transparent) !important; }'
            ;(document.head || document.documentElement).appendChild(style)
        }
        document.querySelectorAll('.' + css).forEach((el) => {
            el.classList.remove(css)
            el.style.removeProperty('--walker-color')
            el.style.removeProperty('--walker-outline-style')
        })
        let first = null
        let found = 0
        for (const locator of locators) {
            let el = null
            try { el = document.querySelector(locator) } catch (e) {}
            if (!el) continue
            found++
            el.classList.add(css)
            el.style.setProperty('--walker-color', color)
            el.style.setProperty('--walker-outline-style', missing ? 'dashed' : 'solid')
            first = first || el
        }
        if (first && scroll) first.scrollIntoView({ block: 'center', behavior: 'smooth' })
        return found
    })
    """
}

extension SemanticShapeVO.TraceStatus {
    /// cssColor is the color of the status in the page
    var cssColor: String {
        switch self {
        case .error: "#ff3b30"
        case .warn: "#ff9500"
        case .info: "#0a84ff"
        case .ok: "#34c759"
        default: "#8e8e93"
        }
    }

    /// color is the color of the status in the app
    var color: Color {
        switch self {
        case .error: .red
        case .warn: .orange
        case .info: .blue
        case .ok: .green
        default: .secondary
        }
    }
}
