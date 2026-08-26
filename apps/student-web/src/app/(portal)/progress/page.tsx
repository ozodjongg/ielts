"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import {
  Card,
  Empty,
  PageHeader,
  Pill,
  Progress,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableWrap,
} from "@/components/ui";
import { api, portalPath } from "@/lib/api";

type ProgressPoint = { service_code: string; day: string; score: number };
type TopicMastery = { subject_code: string; attempts: number; correct: number; mastery: number; updated_at: string };
type ProgressResponse = { items: ProgressPoint[]; topic_mastery: TopicMastery[] };
type EnglishHistory = {
  id: string;
  service_code: string;
  status: string;
  auto_score: number;
  final_score?: number | null;
  level?: string | null;
  readiness?: number | null;
  started_at: string;
  finished_at?: string | null;
};
type SATHistory = {
  id: string;
  status: string;
  raw_correct: number;
  percent?: number | null;
  estimated_sat_score?: number | null;
  started_at: string;
  finished_at?: string | null;
};

export default function ProgressPage() {
  const auth = useAuth();
  const [progress, setProgress] = useState<ProgressResponse>({ items: [], topic_mastery: [] });
  const [english, setEnglish] = useState<EnglishHistory[]>([]);
  const [sat, setSat] = useState<SATHistory[]>([]);

  useEffect(() => {
    if (!auth.session) return;
    let active = true;
    void Promise.all([
      api<ProgressResponse>(portalPath("student", "assessment", "/progress"), auth.session.access_token),
      api<{ items: EnglishHistory[] }>(portalPath("student", "assessment", "/history"), auth.session.access_token),
      api<{ items: SATHistory[] }>(portalPath("student", "sat", "/history"), auth.session.access_token),
    ])
      .then(([progressResponse, englishResponse, satResponse]) => {
        if (!active) return;
        setProgress({
          items: progressResponse.items ?? [],
          topic_mastery: progressResponse.topic_mastery ?? [],
        });
        setEnglish(englishResponse.items ?? []);
        setSat(satResponse.items ?? []);
      })
      .catch((error: unknown) => {
        if (active) toast.error(error instanceof Error ? error.message : "Progress data could not be loaded");
      });
    return () => {
      active = false;
    };
  }, [auth.session]);

  return (
    <>
      <PageHeader title="Progress" subtitle="Academic results are separate from Rush reward points." />
      <div className="grid grid-3 section">
        <Card className="p-6"><div className="muted">Current level</div><div className="metric">{auth.profile?.current_level ?? "A1"}</div></Card>
        <Card className="p-6"><div className="muted">English attempts</div><div className="metric">{english.length}</div></Card>
        <Card className="p-6"><div className="muted">SAT attempts</div><div className="metric">{sat.length}</div></Card>
      </div>

      <Card className="section p-6">
        <h3 className="text-lg font-semibold">Topic mastery</h3>
        <p className="muted mt-1">Lowest mastery topics are shown first so you can focus on weak areas.</p>
        <div className="stack section">
          {progress.topic_mastery.length === 0 ? <Empty>No topic mastery data yet.</Empty> : progress.topic_mastery.map((item) => {
            const percent = Math.round(Number(item.mastery) * 100);
            return (
              <div key={item.subject_code} className="rounded-lg border border-[var(--border)] p-4">
                <div className="row-between">
                  <div><b>{item.subject_code}</b><div className="muted text-xs">{item.correct}/{item.attempts} correct</div></div>
                  <Pill>{percent}%</Pill>
                </div>
                <Progress className="mt-3" value={percent} />
              </div>
            );
          })}
        </div>
      </Card>

      <div className="grid grid-2 section">
        <div>
          <h3 className="mb-3 text-lg font-semibold">English history</h3>
          {english.length === 0 ? <Empty>No completed English attempts yet.</Empty> : (
            <TableWrap>
              <Table>
                <TableHeader><TableRow><TableHead>Service</TableHead><TableHead>Score</TableHead><TableHead>Level</TableHead><TableHead>Date</TableHead></TableRow></TableHeader>
                <TableBody>{english.slice(0, 20).map((item) => <TableRow key={item.id}><TableCell>{item.service_code}</TableCell><TableCell>{item.final_score ?? item.auto_score ?? "—"}</TableCell><TableCell>{item.level ?? "—"}</TableCell><TableCell>{new Date(item.started_at).toLocaleDateString()}</TableCell></TableRow>)}</TableBody>
              </Table>
            </TableWrap>
          )}
        </div>
        <div>
          <h3 className="mb-3 text-lg font-semibold">SAT history</h3>
          {sat.length === 0 ? <Empty>No completed SAT attempts yet.</Empty> : (
            <TableWrap>
              <Table>
                <TableHeader><TableRow><TableHead>Correct</TableHead><TableHead>Percent</TableHead><TableHead>Estimate</TableHead><TableHead>Date</TableHead></TableRow></TableHeader>
                <TableBody>{sat.slice(0, 20).map((item) => <TableRow key={item.id}><TableCell>{item.raw_correct}</TableCell><TableCell>{item.percent == null ? "—" : `${Number(item.percent).toFixed(1)}%`}</TableCell><TableCell>{item.estimated_sat_score ?? "—"}</TableCell><TableCell>{new Date(item.started_at).toLocaleDateString()}</TableCell></TableRow>)}</TableBody>
              </Table>
            </TableWrap>
          )}
        </div>
      </div>

      <Card className="section p-6">
        <h3 className="text-lg font-semibold">Daily score trend</h3>
        <div className="section">
          {progress.items.length === 0 ? <Empty>No score trend data yet.</Empty> : (
            <TableWrap>
              <Table>
                <TableHeader><TableRow><TableHead>Date</TableHead><TableHead>Service</TableHead><TableHead>Average score</TableHead></TableRow></TableHeader>
                <TableBody>{progress.items.slice(-30).reverse().map((item, index) => <TableRow key={`${item.service_code}-${item.day}-${index}`}><TableCell>{new Date(item.day).toLocaleDateString()}</TableCell><TableCell>{item.service_code}</TableCell><TableCell>{Number(item.score).toFixed(1)}</TableCell></TableRow>)}</TableBody>
              </Table>
            </TableWrap>
          )}
        </div>
      </Card>
    </>
  );
}
