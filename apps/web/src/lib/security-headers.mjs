/**
 * @param {string} nonce
 * @returns {string}
 */
export function buildContentSecurityPolicy(nonce, uploadOrigin = "") {
  const storageOrigin = parseMediaUploadOrigin(uploadOrigin);
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
    storageOrigin ? "connect-src 'self' " + storageOrigin : "connect-src 'self'",
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
export function getDocumentSecurityHeaders(nonce, protocol, environment, uploadOrigin = "") {
  const headers = [{
    key: "Content-Security-Policy",
    value: buildContentSecurityPolicy(nonce, uploadOrigin),
  }];

  if (environment === "production" && protocol === "https:") {
    headers.push({
      key: "Strict-Transport-Security",
      value: "max-age=31536000",
    });
  }

  return headers;
}


/** @param {string | undefined} value */
export function parseMediaUploadOrigin(value) {
  if (!value || value.includes("*")) return "";
  try {
    const url = new URL(value);
    if (!/^(?:[a-z0-9.-]+|\[[a-f0-9:]+\])$/.test(url.hostname)) return "";
    const local = url.protocol === "http:" && (/^127(?:\.\d{1,3}){3}$/.test(url.hostname) || url.hostname === "[::1]");
    return (url.protocol === "https:" || local) && value === url.origin ? url.origin : "";
  } catch {
    return "";
  }
}