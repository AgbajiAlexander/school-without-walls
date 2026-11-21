import assert from "node:assert/strict";
import test from "node:test";
import {
  buildContentSecurityPolicy,
  getBaselineSecurityHeaders,
  getDocumentSecurityHeaders,
} from "./security-headers.mjs";

test("CSP includes the request nonce and restricts application resources", () => {
  const nonce = "Y3VycmljdWx1bS1zYWZlLW5vbmNl";
  const policy = buildContentSecurityPolicy(nonce);

  assert.match(policy, new RegExp(`script-src 'self' 'nonce-${nonce}'`));
  assert.match(policy, new RegExp(`style-src 'self' 'nonce-${nonce}'`));
  for (const directive of [
    "default-src 'self'",
    "base-uri 'self'",
    "object-src 'none'",
    "frame-ancestors 'none'",
    "form-action 'self'",
    "style-src-attr 'none'",
    "img-src 'self' data:",
    "font-src 'self'",
    "connect-src 'self'",
    "frame-src 'none'",
    "manifest-src 'self'",
    "worker-src 'self'",
  ]) {
    assert.ok(policy.includes(directive), `missing CSP directive: ${directive}`);
  }
  assert.doesNotMatch(policy, /unsafe-inline|unsafe-eval/);
});

test("CSP rejects invalid nonce values", () => {
  assert.throws(() => buildContentSecurityPolicy(""));
  assert.throws(() => buildContentSecurityPolicy("bad nonce; script-src *"));
});

test("baseline headers retain the required browser protections", () => {
  assert.deepEqual(getBaselineSecurityHeaders(), [
    { key: "Referrer-Policy", value: "no-referrer" },
    { key: "X-Content-Type-Options", value: "nosniff" },
    { key: "X-Frame-Options", value: "DENY" },
    { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), payment=(), usb=()" },
  ]);
});

test("HSTS is limited to production HTTPS document responses", () => {
  const nonce = "Y3VycmljdWx1bS1zYWZlLW5vbmNl";
  const productionHTTPS = getDocumentSecurityHeaders(nonce, "https:", "production");
  const productionHTTP = getDocumentSecurityHeaders(nonce, "http:", "production");
  const developmentHTTPS = getDocumentSecurityHeaders(nonce, "https:", "development");

  assert.ok(productionHTTPS.some(({ key }) => key === "Strict-Transport-Security"));
  assert.ok(productionHTTP.every(({ key }) => key !== "Strict-Transport-Security"));
  assert.ok(developmentHTTPS.every(({ key }) => key !== "Strict-Transport-Security"));
});


test("CSP accepts only a single explicit media upload origin", () => {
  const nonce = "YWJj";
  assert.ok(buildContentSecurityPolicy(nonce, "https://storage.example.test:9443")
    .split("; ").includes("connect-src 'self' https://storage.example.test:9443"));
  for (const value of ["*", "https://*", "https://*.example.test", "https://storage.example.test/path", "https://user@storage.example.test",
    "https://storage.example.test; script-src *", "http://storage.example.test"]) {
    assert.ok(buildContentSecurityPolicy(nonce, value).split("; ").includes("connect-src 'self'"));
  }
});