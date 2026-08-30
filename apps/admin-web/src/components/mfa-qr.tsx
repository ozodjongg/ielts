"use client";

import { useMemo } from "react";

// Vendored MIT QR encoder. It runs entirely in the browser so the TOTP secret
// embedded in the otpauth URI is never disclosed to a third-party QR service.
// eslint-disable-next-line @typescript-eslint/no-require-imports
const QRCode = require("../lib/vendor/qrcode/index.js");
// eslint-disable-next-line @typescript-eslint/no-require-imports
const QRErrorCorrectLevel = require("../lib/vendor/qrcode/QRErrorCorrectLevel.js");

export function MfaQr({ value, size = 220 }: { value: string; size?: number }) {
  const qr = useMemo(() => {
    const code = new QRCode(0, QRErrorCorrectLevel.M);
    code.addData(value);
    code.make();
    const count = code.getModuleCount() as number;
    const quiet = 4;
    let path = "";
    for (let row = 0; row < count; row += 1) {
      for (let col = 0; col < count; col += 1) {
        if (code.isDark(row, col)) path += `M${col + quiet} ${row + quiet}h1v1h-1z`;
      }
    }
    return { count: count + quiet * 2, path };
  }, [value]);

  return (
    <svg
      aria-label="Authenticator QR code"
      role="img"
      width={size}
      height={size}
      viewBox={`0 0 ${qr.count} ${qr.count}`}
      shapeRendering="crispEdges"
      style={{ background: "white", padding: 8, borderRadius: 16, maxWidth: "100%", height: "auto" }}
    >
      <rect width="100%" height="100%" fill="white" />
      <path d={qr.path} fill="black" />
    </svg>
  );
}
