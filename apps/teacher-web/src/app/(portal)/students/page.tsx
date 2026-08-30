"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, Field, Input, PageHeader, Pill, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap, Textarea } from "@/components/ui";
import { api, json, portalPath } from "@/lib/api";

type Student = { user_id: string; full_name: string; email: string; current_level?: string | null; status: string };
type Lexeme = { index: number; english: string; uzbek: unknown; cefr: string };
type Assigned = { id: string; note?: string; due_at?: string | null; word: Lexeme };

function translations(value: unknown) {
  if (Array.isArray(value)) return value.map(String).join(", ");
  if (typeof value === "string") return value;
  if (value && typeof value === "object") return Object.values(value as Record<string, unknown>).map(String).join(", ");
  return "—";
}

export default function Students() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [items, setItems] = useState<Student[]>([]);
  const [student, setStudent] = useState<Student | null>(null);
  const [query, setQuery] = useState("");
  const [found, setFound] = useState<Lexeme[]>([]);
  const [selected, setSelected] = useState<Lexeme[]>([]);
  const [assigned, setAssigned] = useState<Assigned[]>([]);
  const [note, setNote] = useState("");
  const [due, setDue] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    if (!token) return;
    const response = await api<{ items: Student[] }>(portalPath("teacher", "tenant", "/students"), token);
    setItems(response.items ?? []);
  }, [token]);

  const loadAssigned = useCallback(async (studentID: string) => {
    if (!token) return;
    const response = await api<{ items: Assigned[] }>(portalPath("teacher", "vocabulary", `/teacher/students/${studentID}/words`), token);
    setAssigned(response.items ?? []);
  }, [token]);

  useEffect(() => { if (token) void load().catch((e: Error) => toast.error(e.message)); }, [load, token]);

  const selectedIndexes = useMemo(() => new Set(selected.map((item) => item.index)), [selected]);

  async function choose(current: Student) {
    setStudent(current);
    setSelected([]);
    setFound([]);
    setQuery("");
    setNote("");
    setDue("");
    try { await loadAssigned(current.user_id); } catch (e: unknown) { toast.error(e instanceof Error ? e.message : "Assigned words could not be loaded"); }
  }

  async function search() {
    if (!token || !query.trim()) return;
    try {
      const response = await api<{ items: Lexeme[] }>(portalPath("teacher", "vocabulary", `/search?q=${encodeURIComponent(query.trim())}&limit=30`), token);
      setFound(response.items ?? []);
    } catch (e: unknown) { toast.error(e instanceof Error ? e.message : "Dictionary search failed"); }
  }

  async function assign() {
    if (!token || !student || selected.length === 0) return;
    setBusy(true);
    try {
      await api(
        portalPath("teacher", "vocabulary", `/teacher/students/${student.user_id}/words`),
        token,
        json("POST", { lexeme_indexes: selected.map((item) => item.index), note: note.trim(), due_at: due ? new Date(due).toISOString() : null }),
      );
      toast.success(`${selected.length} word(s) assigned to ${student.full_name}`);
      setSelected([]);
      setNote("");
      setDue("");
      await loadAssigned(student.user_id);
    } catch (e: unknown) { toast.error(e instanceof Error ? e.message : "Words could not be assigned"); }
    finally { setBusy(false); }
  }

  return (
    <>
      <PageHeader title="Students" subtitle="View center students and assign extra vocabulary directly to an individual student. Only teachers can perform vocabulary assignments." />
      <div className="section">
        {!items.length ? <Empty>No students.</Empty> : (
          <TableWrap><Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Email</TableHead><TableHead>Level</TableHead><TableHead>Status</TableHead><TableHead /></TableRow></TableHeader><TableBody>
            {items.map((item) => <TableRow key={item.user_id}>
              <TableCell><b>{item.full_name}</b><div className="mono muted">{item.user_id}</div></TableCell>
              <TableCell>{item.email}</TableCell>
              <TableCell><Pill>{item.current_level || "—"}</Pill></TableCell>
              <TableCell><Pill tone={item.status === "active" ? "ok" : "bad"}>{item.status}</Pill></TableCell>
              <TableCell><Button variant="outline" disabled={item.status !== "active"} onClick={() => void choose(item)}>Assign words</Button></TableCell>
            </TableRow>)}
          </TableBody></Table></TableWrap>
        )}
      </div>

      {student ? (
        <Card className="section p-6">
          <div className="row-between gap-4"><div><h2 className="text-xl font-semibold">Extra vocabulary · {student.full_name}</h2><p className="muted mt-1">Selected words immediately enter the student’s spaced-review queue.</p></div><Pill>{selected.length} selected</Pill></div>
          <div className="grid grid-2 section">
            <div>
              <Field label="Find dictionary words">
                <div className="row"><Input value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") void search(); }} placeholder="Search English word" /><Button onClick={() => void search()}>Search</Button></div>
              </Field>
              <div className="stack mt-4" style={{ maxHeight: 320, overflow: "auto" }}>
                {found.map((word) => {
                  const active = selectedIndexes.has(word.index);
                  return <button type="button" className="question-option" key={word.index} onClick={() => setSelected((current) => active ? current.filter((x) => x.index !== word.index) : [...current, word])}>
                    <b>{word.english}</b> · {translations(word.uzbek)} · {word.cefr}{active ? " ✓" : ""}
                  </button>;
                })}
              </div>
            </div>
            <div className="stack">
              <Field label="Optional note"><Textarea value={note} onChange={(e) => setNote(e.target.value)} placeholder="Focus on these words before the next lesson." /></Field>
              <Field label="Optional deadline"><Input type="datetime-local" value={due} onChange={(e) => setDue(e.target.value)} /></Field>
              <Button disabled={busy || selected.length === 0} onClick={() => void assign()}>{busy ? "Assigning…" : "Assign selected words"}</Button>
            </div>
          </div>
          <div className="divider" />
          <h3 className="font-semibold">Previously assigned</h3>
          {!assigned.length ? <div className="muted mt-3">No extra words assigned yet.</div> : <div className="grid grid-2 mt-4">{assigned.slice(0, 20).map((entry) => <div key={entry.id} className="rounded-md border border-[var(--border)] p-3"><div className="row-between"><b>{entry.word.english}</b><Pill>{entry.word.cefr}</Pill></div><div className="muted mt-1">{translations(entry.word.uzbek)}</div>{entry.note ? <div className="mt-2 text-sm">{entry.note}</div> : null}</div>)}</div>}
        </Card>
      ) : null}
    </>
  );
}
