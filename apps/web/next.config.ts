import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  async rewrites() {
    // Proxy API calls through Next.js for local development / same-origin BFF pattern
    const apiUrl = process.env.API_INTERNAL_URL || "http://localhost:8080";
    return [
      {
        source: "/api/v1/:path*",
        destination: `${apiUrl}/api/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
