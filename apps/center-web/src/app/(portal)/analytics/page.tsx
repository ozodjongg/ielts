"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Card, Empty, PageHeader, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap } from "@/components/ui";

type Overview = { attempts_this_month?: number; completions_this_month?: number; active_students?: number };
type Activity = { event_id: string; occurred_at: string; event_type: string; service_code?: string | null; organization_id?: string | null; student_user_id?: string | null };

export default function AnalyticsPage() {
  const auth = useAuth();
  const [overview, setOverview] = useState<Overview>({});
  const [activity, setActivity] = useState<Activity[]>([]);

  useEffect(() => {
    if (!auth.session) return;
    let active = true;
    void Promise.all([
      api<Overview>(portalPath("center", "analytics", "/overview"), auth.session.access_token),
      api<{ items: Activity[] }>(portalPath("center", "analytics", "/activity"), auth.session.access_token),
    ]).then(([o, x]) => {
      if (!active) return;
      setOverview(o); setActivity(x.items || []);
    }).catch((error: Error) => { if (active) toast.error(error.message); });
    return () => { active = false; };
  }, [auth.session]);

  return <>
    <PageHeader title="Analytics" subtitle="Center-scoped activity with tenant isolation enforced by the backend." />
    <div className="grid grid-3 section">
      <Card><div className="muted">Attempts</div><div className="metric">{overview.attempts_this_month ?? 0}</div></Card>
      <Card><div className="muted">Active students</div><div className="metric">{overview.active_students ?? 0}</div></Card>
      <Card><div className="muted">Completions</div><div className="metric">{overview.completions_this_month ?? 0}</div></Card>
    </div>
    <div className="section">{activity.length === 0 ? <Empty>No activity events yet.</Empty> : <TableWrap><Table><TableHeader><TableRow><TableHead>Time</TableHead><TableHead>Event</TableHead><TableHead>Service</TableHead><TableHead>Organization</TableHead><TableHead>Student</TableHead></TableRow></TableHeader><TableBody>{activity.map((item) => <TableRow key={item.event_id}><TableCell>{new Date(item.occurred_at).toLocaleString()}</TableCell><TableCell>{item.event_type}</TableCell><TableCell>{item.service_code || "system"}</TableCell><TableCell className="mono">{String(item.organization_id || "—").slice(0, 12)}</TableCell><TableCell className="mono">{String(item.student_user_id || "—").slice(0, 12)}</TableCell></TableRow>)}</TableBody></Table></TableWrap>}</div>
  </>;
}
