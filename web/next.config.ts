import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin();

const nextConfig: NextConfig = {
  output: "standalone",
  // Pin the root to web/, or a lockfile higher up (e.g. in the home folder) moves server.js inside .next/standalone.
  outputFileTracingRoot: __dirname,
  poweredByHeader: false,
  async rewrites() {
    // Only for `next dev`: send /api to a Go server on :8080. In Docker, Caddy does this.
    return process.env.NODE_ENV === "development"
      ? [{ source: "/api/:path*", destination: "http://localhost:8080/api/:path*" }]
      : [];
  },
};

export default withNextIntl(nextConfig);
