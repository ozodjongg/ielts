"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Card, Empty, PageHeader, Pill, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap } from "@/components/ui";
import { api, portalPath } from "@/lib/api";

type Leader = { rank: number; student_user_id: string; points: number; last_activity?: string };
type MyPoints = { total_points: number; by_service: Record<string, number> };

export default function LeaderboardPage() {
  const auth = useAuth();
  const [items, setItems] = useState<Leader[]>([]);
  const [me, setMe] = useState<MyPoints>({ total_points: 0, by_service: {} });

  useEffect(() => {
    if (!auth.session) return;
    let active = true;
    void Promise.all([
      api<{ items: Leader[] }>(portalPath("student", "points", "/leaderboard"), auth.session.access_token),
      api<MyPoints>(portalPath("student", "points", "/me"), auth.session.access_token),
    ])
      .then(([leaders, mine]) => {
        if (!active) return;
        setItems(leaders.items ?? []);
        setMe(mine);
      })
      .catch((error: unknown) => {
        if (active) toast.error(error instanceof Error ? error.message : "Leaderboard could not be loaded");
      });
    return () => {
      active = false;
    };
  }, [auth.session]);

  return (
    <>
      <PageHeader title="Rush leaderboard" subtitle="Harder questions can award more reward points; academic scores never change." />
      <Card className="section p-6">
        <div className="row-between">
          <div>
            <div className="muted">Your total</div>
            <div className="metric">{me.total_points ?? 0}</div>
          </div>
          <Pill>Rush</Pill>
        </div>
      </Card>
      <div className="section">
        {items.length === 0 ? (
          <Empty>No points have been awarded yet.</Empty>
        ) : (
          <TableWrap>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Rank</TableHead>
                  <TableHead>Student</TableHead>
                  <TableHead>Points</TableHead>
                  <TableHead>Last activity</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.student_user_id}>
                    <TableCell><b>#{item.rank}</b></TableCell>
                    <TableCell>{item.student_user_id === auth.profile?.user_id ? "You" : <span className="mono">{item.student_user_id.slice(0, 12)}…</span>}</TableCell>
                    <TableCell>{item.points}</TableCell>
                    <TableCell>{item.last_activity ? new Date(item.last_activity).toLocaleString() : "—"}</TableCell>
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
