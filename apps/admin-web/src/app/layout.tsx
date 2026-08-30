import type { Metadata, Viewport } from "next";
import "./globals.css";
import { AuthProvider } from "@/components/auth-provider";
import { ThemeProvider, ThemeToaster } from "@/components/theme-provider";

export const metadata: Metadata = {
  title: { default: "IELTS Platform Admin", template: "%s · IELTS Platform Admin" },
  description: "Secure administration for the IELTS learning platform.",
  applicationName: "IELTS Platform",
  category: "education",
  formatDetection: { email: false, address: false, telephone: false },
  robots: {
    index: false,
    follow: false,
    noarchive: true,
    googleBot: { index: false, follow: false, noimageindex: true },
  },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  colorScheme: "light dark",
  themeColor: "#ffffff",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="uz" suppressHydrationWarning>
      <body className="portal-admin">
        <ThemeProvider>
          <AuthProvider portal="admin" expectedRole="admin">{children}</AuthProvider>
          <ThemeToaster />
        </ThemeProvider>
      </body>
    </html>
  );
}
