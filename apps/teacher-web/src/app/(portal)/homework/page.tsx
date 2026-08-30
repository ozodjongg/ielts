"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, Field, Input, PageHeader, Pill, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap, Textarea } from "@/components/ui";

type Group = { id: string; name: string; level?: string | null };
type Student = { student_user_id?: string; user_id?: string; full_name?: string; email?: string; current_level?: string | null };
type Lexeme = { index: number; english: string; uzbek: string[]; cefr: string };
type HW = { id: string; title: string; instructions: string; due_at?: string | null; created_at: string; word_count: number; student_count: number; completed_count: number };

export default function Homework() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [groups, setGroups] = useState<Group[]>([]);
  const [groupID, setGroupID] = useState("");
  const [students, setStudents] = useState<Student[]>([]);
  const [selectedStudents, setSelectedStudents] = useState<string[]>([]);
  const [q, setQ] = useState("");
  const [found, setFound] = useState<Lexeme[]>([]);
  const [selectedWords, setSelectedWords] = useState<Lexeme[]>([]);
  const [title, setTitle] = useState("");
  const [instructions, setInstructions] = useState("");
  const [due, setDue] = useState("");
  const [items, setItems] = useState<HW[]>([]);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    if (!token) return;
    const [g, h] = await Promise.all([
      api<{ items: Group[] }>(portalPath("teacher", "tenant", "/groups"), token),
      api<{ items: HW[] }>(portalPath("teacher", "vocabulary", "/teacher/homework"), token),
    ]);
    setGroups(g.items || []); setItems(h.items || []);
  }, [token]);

  const loadStudents = useCallback(async (id: string) => {
    if (!token || !id) { setStudents([]); return; }
    const result = await api<{ items: Student[] }>(portalPath("teacher", "tenant", `/groups/${id}/students`), token);
    setStudents(result.items || []);
  }, [token]);

  useEffect(() => { void load().catch((e: Error) => toast.error(e.message)); }, [load]);
  useEffect(() => { setSelectedStudents([]); void loadStudents(groupID).catch((e: Error) => toast.error(e.message)); }, [groupID, loadStudents]);
  const wordIds = useMemo(() => new Set(selectedWords.map((w) => w.index)), [selectedWords]);

  async function search() {
    if (!token || !q.trim()) return;
    try {
      const result = await api<{ items: Lexeme[] }>(portalPath("teacher", "vocabulary", `/search?q=${encodeURIComponent(q.trim())}&limit=20`), token);
      setFound(result.items || []);
    } catch (e: any) { toast.error(e.message); }
  }

  async function create() {
    if (!token) return;
    if (!groupID || !title.trim() || !selectedStudents.length || !selectedWords.length) { toast.error("Group, title, students and words are required"); return; }
    setBusy(true);
    try {
      await api(portalPath("teacher", "vocabulary", "/teacher/homework"), token, json("POST", { title: title.trim(), instructions: instructions.trim(), due_at: due ? new Date(due).toISOString() : null, lexeme_indexes: selectedWords.map((w) => w.index), student_user_ids: selectedStudents }));
      toast.success("Homework assigned"); setTitle(""); setInstructions(""); setDue(""); setSelectedStudents([]); setSelectedWords([]); await load();
    } catch (e: any) { toast.error(e.message); } finally { setBusy(false); }
  }

  return <>
    <PageHeader title="Vocabulary homework" subtitle="Avval o‘zingizga biriktirilgan groupni tanlang. Homework faqat shu group studentlariga beriladi; backend ham bu ruxsatni majburiy tekshiradi." />
    <div className="grid grid-2 section">
      <Card>
        <h3>1. My group & students</h3>
        <Field label="Assigned group"><Select value={groupID} onChange={(e) => setGroupID(e.target.value)}><option value="">Select group…</option>{groups.map((group) => <option key={group.id} value={group.id}>{group.name}{group.level ? ` · ${group.level}` : ""}</option>)}</Select></Field>
        {!groupID ? <div className="muted section">Center sizga biriktirgan groupni tanlang.</div> : <div className="stack section" style={{ maxHeight: 300, overflow: "auto" }}>{students.length === 0 ? <div className="muted">Bu groupda student yo‘q.</div> : students.map((student) => { const id = student.student_user_id || student.user_id || ""; return <label className="row" key={id}><input type="checkbox" checked={selectedStudents.includes(id)} onChange={(e) => setSelectedStudents((current) => e.target.checked ? [...current, id] : current.filter((x) => x !== id))} /><span>{student.full_name || student.email || id}</span>{student.current_level ? <Pill>{student.current_level}</Pill> : null}</label>; })}</div>}
        {students.length ? <div className="row"><Button variant="outline" onClick={() => setSelectedStudents(students.map((student) => student.student_user_id || student.user_id || "").filter(Boolean))}>Select all in group</Button><Button variant="ghost" onClick={() => setSelectedStudents([])}>Clear</Button></div> : null}
      </Card>
      <Card>
        <h3>2. Words</h3>
        <div className="row"><Input value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") void search(); }} placeholder="Search English word" /><Button onClick={search}>Search</Button></div>
        <div className="stack section" style={{ maxHeight: 260, overflow: "auto" }}>{found.map((word) => <button className="question-option" key={word.index} onClick={() => setSelectedWords((current) => wordIds.has(word.index) ? current.filter((x) => x.index !== word.index) : [...current, word])}><b>{word.english}</b> · {Array.isArray(word.uzbek) ? word.uzbek.join(", ") : String(word.uzbek)} · {word.cefr}{wordIds.has(word.index) ? " ✓" : ""}</button>)}</div>
        <div className="muted">Selected: {selectedWords.length}</div>
      </Card>
    </div>
    <Card className="section"><h3>3. Homework details</h3><div className="grid grid-2"><Field label="Title"><Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Week 4 academic vocabulary" /></Field><Field label="Due date/time"><Input type="datetime-local" value={due} onChange={(e) => setDue(e.target.value)} /></Field></div><Field label="Instructions"><Textarea value={instructions} onChange={(e) => setInstructions(e.target.value)} placeholder="Review each word and complete spaced repetitions." /></Field><Button className="accent section" onClick={create} disabled={busy || !groupID}>{busy ? "Assigning…" : "Assign homework"}</Button></Card>
    <div className="section">{!items.length ? <Empty>No homework yet.</Empty> : <TableWrap><Table><TableHeader><TableRow><TableHead>Homework</TableHead><TableHead>Words</TableHead><TableHead>Students</TableHead><TableHead>Completed</TableHead><TableHead>Due</TableHead></TableRow></TableHeader><TableBody>{items.map((h) => <TableRow key={h.id}><TableCell><b>{h.title}</b><div className="muted">{h.instructions}</div></TableCell><TableCell>{h.word_count}</TableCell><TableCell>{h.student_count}</TableCell><TableCell>{h.completed_count}/{h.student_count}</TableCell><TableCell>{h.due_at ? new Date(h.due_at).toLocaleString() : "—"}</TableCell></TableRow>)}</TableBody></Table></TableWrap>}</div>
  </>;
}
