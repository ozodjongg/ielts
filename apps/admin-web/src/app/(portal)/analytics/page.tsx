"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import {
  Card,
  Empty,
  PageHeader,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableWrap,
} from "@/components/ui";

type Overview = {
  events_this_month?: number;
  active_students?: number;
  active_centers?: number;
};

type Activity = {
  event_id: string;
  occurred_at: string;
  event_type: string;
  service_code?: string | null;
  organization_id?: string | null;
  student_user_id?: string | null;
};

export default function AnalyticsPage() {
  const auth = useAuth();
  const [overview, setOverview] = useState<Overview>({});
  const [activity, setActivity] = useState<Activity[]>([]);

  useEffect(() => {
    if (!auth.session) return;
    let active = true;
    void Promise.all([
      api<Overview>(portalPath("admin", "analytics", "/overview"), auth.session.access_token),
      api<{ items: Activity[] }>(portalPath("admin", "analytics", "/activity"), auth.session.access_token),
    ])
      .then(([nextOverview, nextActivity]) => {
        if (!active) return;
        setOverview(nextOverview);
        setActivity(nextActivity.items || []);
      })
      .catch((error: Error) => {
        if (active) toast.error(error.message);
      });
    return () => {
      active = false;
    };
  }, [auth.session]);

  return (
    <>
      <PageHeader title="Analytics" subtitle="Cross-tenant event analytics and operational activity." />
      <div className="grid grid-3 section">
        <Card><div className="muted">Events this month</div><div className="metric">{overview.events_this_month ?? 0}</div></Card>
        <Card><div className="muted">Active students</div><div className="metric">{overview.active_students ?? 0}</div></Card>
        <Card><div className="muted">Active centers</div><div className="metric">{overview.active_centers ?? 0}</div></Card>
      </div>
      <div className="section">
        {activity.length === 0 ? <Empty>No activity events yet.</Empty> : (
          <TableWrap>
            <Table>
              <TableHeader><TableRow><TableHead>Time</TableHead><TableHead>Event</TableHead><TableHead>Service</TableHead><TableHead>Organization</TableHead><TableHead>Student</TableHead></TableRow></TableHeader>
              <TableBody>
                {activity.map((item) => (
                  <TableRow key={item.event_id}>
                    <TableCell>{new Date(item.occurred_at).toLocaleString()}</TableCell>
                    <TableCell>{item.event_type}</TableCell>
                    <TableCell>{item.service_code || "system"}</TableCell>
                    <TableCell className="mono">{String(item.organization_id || "—").slice(0, 12)}</TableCell>
                    <TableCell className="mono">{String(item.student_user_id || "—").slice(0, 12)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableWrap>
        )}
      </div>
    </>
  );
}
