import { createServer } from "node:https";
import { request } from "node:http";
import { readFileSync } from "node:fs";

const certificatePath = process.env.E2E_TLS_CERT;
const keyPath = process.env.E2E_TLS_KEY;
if (!certificatePath || !keyPath) {
  throw new Error("E2E_TLS_CERT and E2E_TLS_KEY are required for browser tests");
}

const server = createServer({
  cert: readFileSync(certificatePath),
  key: readFileSync(keyPath),
}, (incoming, outgoing) => {
  const upstream = request({
    hostname: "127.0.0.1",
    port: Number(process.env.E2E_WEB_PORT ?? 3000),
    method: incoming.method,
    path: incoming.url,
    headers: {
      ...incoming.headers,
      host: incoming.headers.host,
      "x-forwarded-proto": "https",
    },
  }, (response) => {
    outgoing.writeHead(response.statusCode ?? 502, response.headers);
    response.pipe(outgoing);
  });

  upstream.on("error", () => {
    if (!outgoing.headersSent) {
      outgoing.writeHead(502, { "Content-Type": "text/plain; charset=utf-8" });
    }
    outgoing.end("Application is unavailable");
  });

  incoming.pipe(upstream);
});

server.listen(Number(process.env.E2E_HTTPS_PORT ?? 3443), "127.0.0.1");
