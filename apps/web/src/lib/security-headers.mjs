/**
 * @param {string} nonce
 * @returns {string}
 */
export function buildContentSecurityPolicy(nonce) {
  if (!/^[A-Za-z0-9+/_=-]+$/.test(nonce)) {
    throw new Error("A valid CSP nonce is required");
  }

  return [
    "default-src 'self'",
    "base-uri 'self'",
    "object-src 'none'",
    "frame-ancestors 'none'",
    "form-action 'self'",
    `script-src 'self' 'nonce-${nonce}'`,
    `style-src 'self' 'nonce-${nonce}'`,
    "style-src-attr 'none'",
    "img-src 'self' data:",
    "font-src 'self'",
    "connect-src 'self'",
    "frame-src 'none'",
    "manifest-src 'self'",
    "worker-src 'self'",
  ].join("; ");
}

export function getBaselineSecurityHeaders() {
  return [
    { key: "Referrer-Policy", value: "no-referrer" },
    { key: "X-Content-Type-Options", value: "nosniff" },
    { key: "X-Frame-Options", value: "DENY" },
    { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), payment=(), usb=()" },
  ];
}

/**
 * @param {string} nonce
 * @param {string} protocol
 * @param {string} environment
 * @returns {Array<{key: string, value: string}>}
 */
export function getDocumentSecurityHeaders(nonce, protocol, environment) {
  const headers = [{
    key: "Content-Security-Policy",
    value: buildContentSecurityPolicy(nonce),
  }];

  if (environment === "production" && protocol === "https:") {
    headers.push({
      key: "Strict-Transport-Security",
      value: "max-age=31536000",
    });
  }

  return headers;
}
