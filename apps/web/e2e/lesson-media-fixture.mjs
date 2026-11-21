import { createHash } from "node:crypto";
import { createServer } from "node:http";
import { createServer as createTCPServer } from "node:net";

const host = "127.0.0.1";
const storageHost = "127.0.0.2";
const storagePort = 18086;
const scannerPort = 18110;
const webOrigin = "https://" + host + ":" + process.env.E2E_HTTPS_PORT;
const fixture = Buffer.from("%PDF-1.4\nFictional counting worksheet\n%%EOF\n");
const checksum = createHash("sha256").update(fixture).digest("base64");
const objects = new Map();
let scanned = false;

const storage = createServer(async (req, res) => {
  try {
    const url = new URL(req.url ?? "/", "http://" + host);
    const path = url.pathname;
    if (path === "/health") return res.end("ready");
    const isQuarantine = /^\/fixture\/quarantine\/[0-9a-f-]{36}\/[0-9a-f-]{36}$/.test(path);
    const isReady = /^\/fixture\/ready\/[0-9a-f-]{36}\/[0-9a-f-]{36}$/.test(path);
    if (!isQuarantine && !isReady) {
      res.writeHead(404).end();
      return;
    }
    if (req.headers.origin === webOrigin) res.setHeader("Access-Control-Allow-Origin", webOrigin);
    if (req.method === "OPTIONS" && isQuarantine && req.headers.origin === webOrigin) {
      res.writeHead(204, {
        "Access-Control-Allow-Methods": "PUT",
        "Access-Control-Allow-Headers": "content-type,x-amz-checksum-sha256",
      }).end();
      return;
    }
    if (req.method === "PUT") {
      const browserUpload = isQuarantine && req.headers.origin === webOrigin &&
        !req.headers.cookie && !req.headers["x-csrf-token"] &&
        url.searchParams.get("X-Amz-Expires") === "120" && url.searchParams.has("X-Amz-Signature");
      const promotion = isReady && scanned && req.headers["if-none-match"] === "*";
      const suppliedChecksum = url.searchParams.get("X-Amz-Checksum-Sha256") ?? req.headers["x-amz-checksum-sha256"];
      if ((!browserUpload && !promotion) || suppliedChecksum !== checksum ||
        Number(req.headers["content-length"]) !== fixture.length || req.headers["content-type"] !== "application/pdf") {
        res.writeHead(400).end();
        return;
      }
      const chunks = [];
      for await (const chunk of req) chunks.push(chunk);
      const payload = Buffer.concat(chunks);
      if (!payload.equals(fixture)) {
        res.writeHead(400).end();
        return;
      }
      if (promotion && objects.has(path)) {
        res.writeHead(412).end();
        return;
      }
      objects.set(path, payload);
      res.writeHead(200).end();
      return;
    }
    if (req.method === "GET" && objects.has(path)) {
      if (isReady && (req.headers.cookie || req.headers["x-csrf-token"] || req.headers.referer ||
        url.searchParams.get("response-content-disposition") !== "attachment" ||
        url.searchParams.get("X-Amz-Expires") !== "120" || !url.searchParams.has("X-Amz-Signature"))) {
        res.writeHead(400).end();
        return;
      }
      res.writeHead(200, {
        "Content-Type": "application/pdf",
        "Content-Length": fixture.length,
        ...(isReady ? { "Content-Disposition": 'attachment; filename="counting.pdf"' } : {}),
      }).end(objects.get(path));
      return;
    }
    res.writeHead(404).end();
  } catch {
    res.writeHead(500).end();
  }
});

const command = Buffer.from("zINSTREAM\0");
const frame = Buffer.alloc(4);
frame.writeUInt32BE(fixture.length);
const expected = Buffer.concat([command, frame, fixture, Buffer.alloc(4)]);
const scanner = createTCPServer((socket) => {
  let envelope = Buffer.alloc(0);
  socket.setTimeout(5000, () => socket.destroy());
  socket.on("error", () => socket.destroy());
  socket.on("data", (chunk) => {
    envelope = Buffer.concat([envelope, chunk]);
    if (envelope.length >= expected.length) {
      scanned = envelope.equals(expected);
      socket.end(scanned ? "stream: OK\0" : "stream: fixture mismatch ERROR\0");
    }
  });
});
scanner.listen(scannerPort, host, () => storage.listen(storagePort, storageHost));
