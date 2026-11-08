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

    /// ElementHTML is an element of the page as indented html - nil if the
    /// locator matched nothing in the page of the web view
    struct ElementHTML: Hashable, Sendable {
        let locator: String
        let html: String?
        /// truncated: the element was too large and is cut off
        let truncated: Bool
    }

    /// html serializes the elements at the locators from the DOM of the web
    /// view, indented, without the marks of the highlight
    func html(locators: [String]) async -> [ElementHTML] {
        guard let webView, !locators.isEmpty,
              let json = try? String(data: JSONEncoder().encode(locators), encoding: .utf8) else { return [] }
        let script = Self.htmlScript + "(\(json))"
        return await withCheckedContinuation { continuation in
            webView.evaluateJavaScript(script) { result, _ in
                // the untyped result stays here, only the Sendable elements leave
                let entries = result as? [[String: Any]] ?? []
                let elements = zip(locators, entries).map { locator, entry in
                    ElementHTML(locator: locator, html: entry["html"] as? String, truncated: entry["truncated"] as? Bool ?? false)
                }
                continuation.resume(returning: elements)
            }
        }
    }

    /// htmlScript serializes elements: one line per element, text and
    /// comment, an element with only a short text on one line, script and
    /// style bodies shortened - at most maxLines lines per element
    private static let htmlScript = """
    (function(locators) {
        const css = 'walker-shape-highlight'
        const voids = new Set(['area','base','br','col','embed','hr','img','input','link','meta','source','track','wbr'])
        const maxLines = 2000
        const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        const attrs = (el) => {
            let out = ''
            for (const a of el.attributes) {
                let value = a.value
                if (a.name === 'class') value = value.split(/\\s+/).filter((c) => c && c !== css).join(' ')
                if (a.name === 'style') value = value.split(';').filter((d) => d.trim() && !d.trim().startsWith('--walker-')).join(';').trim()
                if ((a.name === 'class' || a.name === 'style') && !value) continue
                // always with the value - alt="" is not the same as no alt
                out += ' ' + a.name + '="' + value.replace(/&/g, '&amp;').replace(/"/g, '&quot;') + '"'
            }
            return out
        }
        const text = (s) => s.replace(/\\s+/g, ' ').trim()
        return locators.map((locator) => {
            let root = null
            try { root = document.querySelector(locator) } catch (e) {}
            if (!root) return {}
            const lines = []
            let truncated = false
            const add = (depth, line) => {
                if (lines.length >= maxLines) { truncated = true; return false }
                lines.push('  '.repeat(depth) + line)
                return true
            }
            const visit = (node, depth) => {
                if (truncated) return
                if (node.nodeType === Node.TEXT_NODE) {
                    const t = text(node.textContent)
                    if (t) add(depth, esc(t))
                    return
                }
                if (node.nodeType === Node.COMMENT_NODE) {
                    add(depth, '<!-- ' + text(node.textContent) + ' -->')
                    return
                }
                if (node.nodeType !== Node.ELEMENT_NODE) return
                const tag = node.tagName.toLowerCase()
                const open = '<' + tag + attrs(node) + '>'
                if (voids.has(tag)) { add(depth, open); return }
                if (tag === 'script' || tag === 'style') {
                    const body = node.textContent.trim()
                    const short = body.length > 200 ? body.slice(0, 200) + ' …' : body
                    add(depth, open + esc(text(short)) + '</' + tag + '>')
                    return
                }
                const children = Array.from(node.childNodes).filter((c) => c.nodeType !== Node.TEXT_NODE || text(c.textContent))
                if (children.length === 0) { add(depth, open + '</' + tag + '>'); return }
                if (children.length === 1 && children[0].nodeType === Node.TEXT_NODE && text(children[0].textContent).length <= 80) {
                    add(depth, open + esc(text(children[0].textContent)) + '</' + tag + '>')
                    return
                }
                if (!add(depth, open)) return
                for (const child of children) visit(child, depth + 1)
                add(depth, '</' + tag + '>')
            }
            visit(root, 0)
            return { html: lines.join('\\n'), truncated: truncated }
        })
    })
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
