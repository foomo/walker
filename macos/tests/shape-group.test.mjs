// Run with node --test macos/tests/shape-group.test.mjs. No browser or network needed.
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const source = await readFile(new URL('../Walker/Views/ShapeWebView.swift', import.meta.url), 'utf8');
const script = source.match(/private static let groupHeaderScript = """([\s\S]*?)"""/);
assert(script, 'Use the JavaScript actually sent to WKWebView');
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const lookup = new AsyncFunction('window', 'fetch', 'header', 'AbortSignal', script[1]);
const page = () => ({ location: { href: 'https://fixture.invalid/catalog' } });

test('uses the configured header and the current browser session', async () => {
  const window = page();
  const value = await lookup(window, async (url, options) => {
    assert.equal(url, window.location.href);
    assert.equal(options.method, 'HEAD');
    assert.equal(options.credentials, 'same-origin');
    assert.equal(options.cache, 'no-store');
    assert(options.signal instanceof AbortSignal);
    return new Response(null, { headers: { 'X-Page-Kind': 'catalog' } });
  }, 'x-page-kind', AbortSignal);
  assert.equal(value, 'catalog');
});

test('reads the new URL after client-side navigation', async () => {
  const window = page();
  const fetch = async url => new Response(null, { headers: { 'X-Group': new URL(url).pathname.slice(1) } });
  assert.equal(await lookup(window, fetch, 'X-Group', AbortSignal), 'catalog');
  window.location.href = 'https://fixture.invalid/article';
  assert.equal(await lookup(window, fetch, 'X-Group', AbortSignal), 'article');
});

test('normalizes directory groups like the crawler', async () => {
  const fetch = async () => new Response(null, { headers: { 'X-Group': 'content/' } });
  assert.equal(await lookup(page(), fetch, 'X-Group', AbortSignal), 'content/index');
});

test('missing headers resolve to the default shape without a second request', async () => {
  let requests = 0;
  const fetch = async () => { requests++; return new Response(null); };
  assert.equal(await lookup(page(), fetch, 'X-Group', AbortSignal), '');
  assert.equal(requests, 1);
});

for (const status of [405, 501]) {
  test(`falls back to GET for HTTP ${status} and cancels the unused body`, async () => {
    const window = page();
    const originalURL = window.location.href;
    const methods = [];
    let signal;
    let cancelled = false;
    const fetch = async (url, options) => {
      assert.equal(url, originalURL, 'fallback must not switch to a newly navigated page');
      methods.push(options.method);
      if (options.method === 'HEAD') {
        signal = options.signal;
        window.location.href = 'https://fixture.invalid/another-page';
        return new Response(null, { status });
      }
      assert.equal(options.signal, signal, 'both requests share one deadline');
      const body = new ReadableStream({ cancel() { cancelled = true; } });
      return new Response(body, { headers: { 'X-Group': 'catalog' } });
    };
    assert.equal(await lookup(window, fetch, 'X-Group', AbortSignal), 'catalog');
    assert.deepEqual(methods, ['HEAD', 'GET']);
    assert(cancelled);
  });
}

test('does not silently use the default shape on an HTTP failure', async () => {
  let requests = 0;
  const fetch = async () => { requests++; return new Response(null, { status: 403 }); };
  await assert.rejects(lookup(page(), fetch, 'X-Group', AbortSignal), /HTTP 403/);
  assert.equal(requests, 1);
});

test('bounds the lookup and propagates a timeout', async () => {
  const controller = new AbortController();
  const deadline = {
    timeout(milliseconds) {
      assert.equal(milliseconds, 10_000);
      queueMicrotask(() => controller.abort(new Error('lookup timed out')));
      return controller.signal;
    },
  };
  const fetch = async (_url, { signal }) => new Promise((_resolve, reject) => {
    signal.addEventListener('abort', () => reject(signal.reason), { once: true });
  });
  await assert.rejects(lookup(page(), fetch, 'X-Group', deadline), /lookup timed out/);
});
