import type { Metadata, Viewport } from "next";
import "./globals.css";
import { AuthProvider } from "@/components/auth-provider";
import { ThemeProvider, ThemeToaster } from "@/components/theme-provider";

export const metadata: Metadata = {
  title: { default: "IELTS Teacher", template: "%s · IELTS Teacher" },
  description: "Secure teacher vocabulary and homework for IELTS Platform.",
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
      <body className="portal-teacher">
        <ThemeProvider>
          <AuthProvider portal="teacher" expectedRole="teacher">{children}</AuthProvider>
          <ThemeToaster />
        </ThemeProvider>
      </body>
    </html>
  );
}
