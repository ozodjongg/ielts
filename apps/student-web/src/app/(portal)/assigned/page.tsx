"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, PageHeader, Pill, Progress } from "@/components/ui";
import { api, json, portalPath } from "@/lib/api";

type Word = {
  index: number;
  english: string;
  uzbek: unknown;
  part_of_speech?: string | null;
  cefr: string;
};

type AssignedWord = {
  id: string;
  created_at: string;
  note?: string;
  due_at?: string | null;
  homework_id?: string | null;
  word: Word;
};

type Homework = {
  id: string;
  title: string;
  instructions?: string;
  due_at?: string | null;
  created_at: string;
  completed_at?: string | null;
};

type AssignedPayload = { words: AssignedWord[]; homework: Homework[] };

function translations(value: unknown) {
  if (Array.isArray(value)) return value.map(String).join(", ");
  if (typeof value === "string") return value;
  if (value && typeof value === "object") return Object.values(value as Record<string, unknown>).map(String).join(", ");
  return "—";
}

function dueLabel(value?: string | null) {
  if (!value) return "No deadline";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "No deadline";
  return date.toLocaleString();
}

export default function AssignedPage() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [data, setData] = useState<AssignedPayload>({ words: [], homework: [] });
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const response = await api<AssignedPayload>(portalPath("student", "vocabulary", "/assigned"), token);
      setData({ words: response.words ?? [], homework: response.homework ?? [] });
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    if (!token) return;
    void load().catch((error: unknown) => toast.error(error instanceof Error ? error.message : "Assignments could not be loaded"));
  }, [load, token]);

  const wordsByHomework = useMemo(() => {
    const map = new Map<string, AssignedWord[]>();
    for (const item of data.words) {
      if (!item.homework_id) continue;
      const current = map.get(item.homework_id) ?? [];
      current.push(item);
      map.set(item.homework_id, current);
    }
    return map;
  }, [data.words]);

  const standalone = useMemo(() => data.words.filter((item) => !item.homework_id), [data.words]);
  const completed = data.homework.filter((item) => item.completed_at).length;
  const progress = data.homework.length ? (completed / data.homework.length) * 100 : 0;

  async function completeHomework(id: string) {
    if (!token) return;
    try {
      await api(portalPath("student", "vocabulary", `/assigned/homework/${id}/complete`), token, json("POST", {}));
      toast.success("Homework marked complete");
      await load();
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Homework could not be completed");
    }
  }

  return (
    <>
      <PageHeader
        title="Teacher assignments"
        subtitle="Vocabulary homework and extra words assigned directly by your teacher. Assigned words are also added to your spaced-review queue."
      />

      {data.homework.length ? (
        <Card className="section p-6">
          <div className="row-between">
            <div>
              <b>Homework overview</b>
              <div className="muted mt-1">{completed} of {data.homework.length} marked complete</div>
            </div>
            <Pill>{data.words.length} assigned words</Pill>
          </div>
          <Progress className="mt-4" value={progress} />
        </Card>
      ) : null}

      <div className="section grid grid-2">
        {data.homework.map((item) => {
          const words = wordsByHomework.get(item.id) ?? [];
          const overdue = item.due_at && !item.completed_at && new Date(item.due_at).getTime() < Date.now();
          return (
            <Card key={item.id} className="p-6">
              <div className="row-between gap-3">
                <h2 className="text-lg font-semibold">{item.title}</h2>
                <Pill>{item.completed_at ? "Completed" : overdue ? "Overdue" : "Active"}</Pill>
              </div>
              {item.instructions ? <p className="mt-3">{item.instructions}</p> : null}
              <div className="row-between mt-3 gap-3">
                <div className="muted">Due: {dueLabel(item.due_at)}</div>
                {!item.completed_at ? <Button variant="outline" onClick={() => void completeHomework(item.id)}>Mark complete</Button> : null}
              </div>
              <div className="divider" />
              <div className="stack">
                {words.length ? words.map((entry) => (
                  <div key={entry.id} className="row-between gap-4 rounded-md border border-[var(--border)] p-3">
                    <div>
                      <b>{entry.word.english}</b>
                      <div className="muted mt-1">{translations(entry.word.uzbek)}</div>
                    </div>
                    <Pill>{entry.word.cefr}</Pill>
                  </div>
                )) : <div className="muted">No vocabulary items are attached to this homework.</div>}
              </div>
            </Card>
          );
        })}
      </div>

      {standalone.length ? (
        <div className="section">
          <PageHeader title="Extra words" subtitle="Additional vocabulary your teacher assigned directly to you." />
          <div className="grid grid-2 section">
            {standalone.map((entry) => (
              <Card key={entry.id} className="p-6">
                <div className="row-between gap-4"><b>{entry.word.english}</b><Pill>{entry.word.cefr}</Pill></div>
                <div className="mt-2">{translations(entry.word.uzbek)}</div>
                {entry.note ? <p className="muted mt-3">{entry.note}</p> : null}
                <div className="muted mt-3">Due: {dueLabel(entry.due_at)}</div>
              </Card>
            ))}
          </div>
        </div>
      ) : null}

      {!loading && data.homework.length === 0 && standalone.length === 0 ? (
        <div className="section"><Empty>Your teacher has not assigned vocabulary homework yet.</Empty></div>
      ) : null}
      {loading ? <div className="section"><Empty>Loading assignments…</Empty></div> : null}
    </>
  );
}
