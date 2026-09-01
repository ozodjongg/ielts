# Local QR encoder

Vendored QR matrix encoder used only to render MFA `otpauth://` values locally in the browser.
The upstream QRCode for JavaScript implementation by Kazuhiko Arase is MIT licensed; see the copyright/license header in `index.js`.
No MFA secret is sent to a third-party QR service.
