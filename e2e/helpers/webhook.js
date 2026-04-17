import { createServer } from 'http';

export async function startWebhookReceiver() {
  const received = [];
  let responder = null;

  const server = createServer((req, res) => {
    let body = '';
    req.on('data', (chunk) => (body += chunk));
    req.on('end', () => {
      let parsed = body;
      try { parsed = JSON.parse(body); } catch {}
      received.push({
        method: req.method,
        path: req.url,
        headers: req.headers,
        body: parsed,
        rawBody: body,
        at: Date.now(),
      });
      if (responder) {
        const { status, body: respBody, headers } = responder(req) || {};
        res.writeHead(status || 200, headers || { 'Content-Type': 'application/json' });
        res.end(respBody || '{"ok":true}');
      } else {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end('{"ok":true}');
      }
    });
  });

  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => {
      const port = server.address().port;
      resolve({
        port,
        url: `http://127.0.0.1:${port}`,
        received,
        setResponder: (fn) => (responder = fn),
        clear: () => (received.length = 0),
        stop: () => new Promise((r) => server.close(() => r())),
        waitFor: async (predicate, timeoutMs = 20_000) => {
          const deadline = Date.now() + timeoutMs;
          while (Date.now() < deadline) {
            const match = received.find(predicate);
            if (match) return match;
            await new Promise((r) => setTimeout(r, 200));
          }
          throw new Error(`webhook not received matching predicate within ${timeoutMs}ms. got ${received.length} requests`);
        },
      });
    });
  });
}
