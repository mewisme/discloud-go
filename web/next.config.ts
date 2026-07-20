import type { NextConfig } from "next";

const API_URL = process.env.API_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  reactCompiler: true,
  output: "standalone",
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${API_URL}/api/:path*` },
      { source: "/f/:path*", destination: `${API_URL}/f/:path*` },
      { source: "/readyz", destination: `${API_URL}/readyz` },
    ];
  },
};

export default nextConfig;
