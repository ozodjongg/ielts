"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Card, Empty, PageHeader, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap } from "@/components/ui";

type Leader = { rank: number; student_user_id: string; points: number; last_activity: string };

export default function PointsPage() {
  const auth = useAuth(); const [items, setItems] = useState<Leader[]>([]);
  useEffect(() => { if (!auth.session) return; let active = true; void api<{ items: Leader[] }>(portalPath("center", "points", "/leaderboard"), auth.session.access_token).then((result) => { if (active) setItems(result.items || []); }).catch((error: Error) => { if (active) toast.error(error.message); }); return () => { active = false; }; }, [auth.session]);
  return <><PageHeader title="Rush Points" subtitle="Reward layer based on question difficulty; academic scores remain unchanged."/><Card className="section"><div className="stack"><span className="muted">Multiplier starts at 1.0× until sufficient attempts exist, then increases for genuinely difficult questions.</span><b>Reward = base points × Rush multiplier</b></div></Card><div className="section">{items.length === 0 ? <Empty>No points have been awarded yet.</Empty> : <TableWrap><Table><TableHeader><TableRow><TableHead>#</TableHead><TableHead>Student</TableHead><TableHead>Points</TableHead><TableHead>Last activity</TableHead></TableRow></TableHeader><TableBody>{items.map((item) => <TableRow key={item.student_user_id}><TableCell>{item.rank}</TableCell><TableCell className="mono">{item.student_user_id}</TableCell><TableCell><b>{item.points}</b></TableCell><TableCell>{new Date(item.last_activity).toLocaleString()}</TableCell></TableRow>)}</TableBody></Table></TableWrap>}</div></>;
}
