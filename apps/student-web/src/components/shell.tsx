"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";
import {
  BookOpenCheck,
  ChartNoAxesCombined,
  Headphones,
  House,
  Languages,
  BrainCircuit,
  Mic2,
  Trophy,
  UserRound,
  ListChecks
} from "lucide-react";
import { useAuth } from "@/components/auth-provider";
import { Button, Pill } from "@/components/ui";
import { ThemeToggle } from "@/components/theme-provider";

const nav = [
  { href: "/", label: "Home", icon: House },
  { href: "/services", label: "Services", icon: BookOpenCheck },
  { href: "/vocabulary", label: "Vocabulary", icon: Languages },
  { href: "/assigned", label: "Assigned", icon: ListChecks },
  { href: "/review", label: "Review", icon: BrainCircuit },
  { href: "/listening", label: "Listening", icon: Headphones },
  { href: "/submissions", label: "Speaking & Writing", icon: Mic2 },
  { href: "/progress", label: "Progress", icon: ChartNoAxesCombined },
  { href: "/leaderboard", label: "Leaderboard", icon: Trophy },
  { href: "/profile", label: "Profile", icon: UserRound }
];

export function PortalShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const auth = useAuth();

  useEffect(() => {
    if (!auth.loading && !auth.profile && pathname !== "/login") {
      router.replace("/login");
    }
  }, [auth.loading, auth.profile, pathname, router]);

  if (auth.loading) {
    return <div className="login-wrap"><div className="muted">IELTS portal yuklanmoqda…</div></div>;
  }
  if (!auth.profile) return null;

  return (
    <div className="shell">
      <aside className="sidebar" aria-label="Primary navigation">
        <div className="brand">
          <span className="brandmark" aria-hidden="true">IELTS</span>
          <div>IELTS Student<div className="muted" style={{ fontSize: 11, fontWeight: 500 }}>Student learning and assessment portal</div></div>
        </div>
        <nav className="nav">
          {nav.map((item) => {
            const Icon = item.icon;
            const active = pathname === item.href;
            return (
              <Link key={item.href} href={item.href} className={active ? "active" : ""} aria-current={active ? "page" : undefined}>
                <Icon size={17} aria-hidden="true" />{item.label}
              </Link>
            );
          })}
        </nav>
        <div className="sidebar-footer">
          <div style={{ fontSize: 13, fontWeight: 650 }}>{auth.profile.full_name}</div>
          <div className="muted" style={{ fontSize: 11, margin: "4px 0 12px" }}>{auth.profile.email}</div>
          <Button variant="outline" onClick={async () => { await auth.logout(); router.replace("/login"); }}>Chiqish</Button>
        </div>
      </aside>
      <main className="main">
        <header className="topbar">
          <b>Your learning workspace</b>
          <div className="row">
            <ThemeToggle />
            <Pill>{auth.profile.current_level || auth.profile.role}</Pill>
          </div>
        </header>
        <div className="content">{children}</div>
      </main>
      <nav className="mobile-nav" aria-label="Mobile navigation">
        {nav.map((item) => <Link key={item.href} href={item.href}>{item.label}</Link>)}
      </nav>
    </div>
  );
}
