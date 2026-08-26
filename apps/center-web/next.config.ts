import type { NextConfig } from "next";

const isProduction = process.env.NODE_ENV === "production";
const rawApiUrl = (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080").trim();
let apiOrigin = "http://localhost:8080";
try { apiOrigin = new URL(rawApiUrl).origin; } catch { /* validated again by the API client */ }

const csp = [
  "default-src 'self'",
  `connect-src 'self' ${apiOrigin} ${isProduction ? "" : "ws: wss:"}`,
  "img-src 'self' data: blob:",
  "font-src 'self' data:",
  `script-src 'self' 'unsafe-inline' ${isProduction ? "" : "'unsafe-eval'"}`,
  "style-src 'self' 'unsafe-inline'",
  `media-src 'self' blob: ${apiOrigin}`,
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "worker-src 'self' blob:",
].filter(Boolean).join("; ");

const securityHeaders = [
  { key: "Content-Security-Policy", value: csp },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Referrer-Policy", value: "no-referrer" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), payment=(), usb=()" },
  { key: "Cross-Origin-Opener-Policy", value: "same-origin" },
  { key: "X-DNS-Prefetch-Control", value: "off" },
  { key: "X-Robots-Tag", value: "noindex, nofollow, noarchive, nosnippet" },
  ...(isProduction ? [{ key: "Strict-Transport-Security", value: "max-age=31536000; includeSubDomains" }] : []),
];

const nextConfig: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  compress: true,
  async headers() {
    return [{ source: "/(.*)", headers: securityHeaders }];
  },
};

export default nextConfig;
