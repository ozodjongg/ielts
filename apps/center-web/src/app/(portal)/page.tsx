"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Card, PageHeader, Pill, Progress } from "@/components/ui";

type Overview = { attempts_this_month?: number; completions_this_month?: number; active_students?: number };
type ServiceLimit = { service_code: string; name: string; used: number; monthly_limit: number; remaining: number; unit: string; enabled: boolean };
type Student = { user_id: string };

export default function CenterOverviewPage() {
  const auth = useAuth(); const [overview, setOverview] = useState<Overview>({}); const [services, setServices] = useState<ServiceLimit[]>([]); const [students, setStudents] = useState<Student[]>([]);
  useEffect(() => { if (!auth.session) return; let active = true; void Promise.all([api<Overview>(portalPath("center","analytics","/overview"),auth.session.access_token),api<{items:ServiceLimit[]}>(portalPath("center","tenant","/services"),auth.session.access_token),api<{items:Student[]}>(portalPath("center","tenant","/students"),auth.session.access_token)]).then(([o,s,st]) => { if (!active) return; setOverview(o); setServices(s.items || []); setStudents(st.items || []); }).catch((error: Error) => { if (active) toast.error(error.message); }); return () => { active = false; }; }, [auth.session]);
  return <><PageHeader title="Center overview" subtitle="Students, monthly quotas, assessments and review activity."/><div className="grid grid-4 section"><Card><div className="muted">Students</div><div className="metric">{students.length}</div></Card><Card><div className="muted">Attempts this month</div><div className="metric">{overview.attempts_this_month ?? 0}</div></Card><Card><div className="muted">Completions</div><div className="metric">{overview.completions_this_month ?? 0}</div></Card><Card><div className="muted">Active students</div><div className="metric">{overview.active_students ?? 0}</div></Card></div><div className="grid grid-2 section">{services.slice(0,8).map((item) => <Card key={item.service_code}><div className="row-between"><b>{item.name}</b><Pill tone={item.enabled ? "ok" : "bad"}>{item.used}/{item.monthly_limit}</Pill></div><div className="section"><Progress value={(item.used / Math.max(1,item.monthly_limit)) * 100}/></div><p className="muted">Remaining: {item.remaining} {item.unit}</p></Card>)}</div></>;
}
