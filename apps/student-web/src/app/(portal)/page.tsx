"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Card, Empty, PageHeader, Pill } from "@/components/ui";
import { api, portalPath } from "@/lib/api";

type Points = { total_points: number; by_service: Record<string, number> };
type Overview = { activity_events: number; by_service: Record<string, number> };
type ServiceLimit = { service_code: string; enabled: boolean; monthly_limit?: number | null; daily_limit?: number | null };

const modules = [
  ["/vocabulary", "Daily Vocabulary", "Level-matched words + spaced repetition"],
  ["/english", "English Tests", "Placement, progress and level-up"],
  ["/listening", "Listening", "Protected private audio assessments"],
  ["/sat", "SAT Math", "English-language SAT-style practice"],
  ["/submissions", "Speaking & Writing", "Submit work for teacher review"],
  ["/leaderboard", "Rush Leaderboard", "Points reward genuine difficulty"],
] as const;

export default function StudentHomePage() {
  const auth = useAuth();
  const [points, setPoints] = useState<Points>({ total_points: 0, by_service: {} });
  const [overview, setOverview] = useState<Overview>({ activity_events: 0, by_service: {} });
  const [services, setServices] = useState<ServiceLimit[]>([]);

  useEffect(() => {
    if (!auth.session) return;
    let active = true;
    void Promise.all([
      api<Points>(portalPath("student", "points", "/me"), auth.session.access_token),
      api<Overview>(portalPath("student", "analytics", "/overview"), auth.session.access_token),
      api<{ items: ServiceLimit[] }>(portalPath("student", "tenant", "/services"), auth.session.access_token),
    ])
      .then(([pointsResponse, overviewResponse, servicesResponse]) => {
        if (!active) return;
        setPoints(pointsResponse);
        setOverview(overviewResponse);
        setServices(servicesResponse.items ?? []);
      })
      .catch((error: unknown) => {
        if (active) toast.error(error instanceof Error ? error.message : "Dashboard data could not be loaded");
      });
    return () => {
      active = false;
    };
  }, [auth.session]);

  const enabledServices = services.filter((item) => item.enabled).length;

  return (
    <>
      <div className="hero">
        <div className="kicker">Current level</div>
        <div className="row-between">
          <div>
            <h1 className="mt-2 text-4xl font-semibold tracking-tight">{auth.profile?.current_level ?? "A1"}</h1>
            <p className="muted mt-2">Daily vocabulary and assignments adapt to your current level.</p>
          </div>
          <Pill tone="ok">Active learner</Pill>
        </div>
      </div>
      <div className="grid grid-3 section">
        <Card className="p-6"><div className="muted">Rush Points</div><div className="metric">{points.total_points ?? 0}</div></Card>
        <Card className="p-6"><div className="muted">Activity this month</div><div className="metric">{overview.activity_events ?? 0}</div></Card>
        <Card className="p-6"><div className="muted">Available services</div><div className="metric">{enabledServices}</div></Card>
      </div>
      <PageHeader title="Continue learning" subtitle="Choose your next module." />
      {services.length > 0 && enabledServices === 0 ? <div className="section"><Empty>Your center has not enabled any learning services yet.</Empty></div> : null}
      <div className="grid grid-3 section">
        {modules.map(([href, title, description]) => (
          <Link key={href} href={href} className="block focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black focus-visible:ring-offset-2">
            <Card className="h-full p-6 transition hover:border-[var(--foreground)]">
              <h3 className="text-base font-semibold">{title}</h3>
              <p className="muted mt-2">{description}</p>
            </Card>
          </Link>
        ))}
      </div>
    </>
  );
}
