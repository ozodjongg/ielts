"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { BookOpenCheck, Headphones, Sigma } from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "@/components/auth-provider";
import { api, json, portalPath } from "@/lib/api";
import { Button, Card, Empty, Field, Input, PageHeader, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap } from "@/components/ui";

type Tab = "english" | "sat" | "listening";
type Group = { id: string; name: string };
type Student = { user_id: string; full_name: string };
type Assignment = { id: string; title?: string; service_code?: string; target_type: string; question_count?: number; created_at?: string; set_id?: string };
type ListeningSet = { id: string; title: string; level?: string | null };

const englishServices = ["placement", "level_upgrade", "progress", "grammar", "ielts_readiness", "weakness", "speaking", "writing", "mock", "final_exit"];
const defaults: Record<string, number> = { placement: 80, level_upgrade: 40, progress: 30, grammar: 40, ielts_readiness: 40, weakness: 30, speaking: 3, writing: 2, mock: 60, final_exit: 60 };

export default function TeacherServicesPage() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [tab, setTab] = useState<Tab>("english");
  const [groups, setGroups] = useState<Group[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [english, setEnglish] = useState<Assignment[]>([]);
  const [sat, setSat] = useState<Assignment[]>([]);
  const [listening, setListening] = useState<Assignment[]>([]);
  const [sets, setSets] = useState<ListeningSet[]>([]);
  const [busy, setBusy] = useState(false);
  const [targetType, setTargetType] = useState<"group" | "student">("group");
  const [targetID, setTargetID] = useState("");
  const [englishForm, setEnglishForm] = useState({ service_code: "progress", title: "Progress test", question_count: 30, from_level: "A1" });
  const [satForm, setSatForm] = useState({ title: "SAT Math practice", question_count: 44 });
  const [setID, setSetID] = useState("");

  const load = useCallback(async () => {
    if (!token) return;
    const [g, s, e, m, l, ls] = await Promise.all([
      api<{ items: Group[] }>(portalPath("teacher", "tenant", "/groups"), token),
      api<{ items: Student[] }>(portalPath("teacher", "tenant", "/students"), token),
      api<{ items: Assignment[] }>(portalPath("teacher", "assessment", "/assignments"), token),
      api<{ items: Assignment[] }>(portalPath("teacher", "sat", "/assignments"), token),
      api<{ items: Assignment[] }>(portalPath("teacher", "listening", "/assignments"), token),
      api<{ items: ListeningSet[] }>(portalPath("teacher", "listening", "/sets"), token),
    ]);
    setGroups(g.items || []); setStudents(s.items || []); setEnglish(e.items || []); setSat(m.items || []); setListening(l.items || []); setSets(ls.items || []);
  }, [token]);

  useEffect(() => { void load().catch((error: Error) => toast.error(error.message)); }, [load]);
  const targets = useMemo(() => targetType === "group" ? groups.map((x) => ({ id: x.id, label: x.name })) : students.map((x) => ({ id: x.user_id, label: x.full_name })), [targetType, groups, students]);
  useEffect(() => { setTargetID(""); }, [targetType]);

  async function createEnglish() {
    if (!token || !targetID) return toast.error("O‘zingizga biriktirilgan group yoki studentni tanlang");
    setBusy(true);
    try {
      const body: Record<string, unknown> = { service_code: englishForm.service_code, title: englishForm.title.trim(), target_type: targetType, target_id: targetID, question_count: englishForm.question_count };
      if (englishForm.service_code === "level_upgrade") {
        const next: Record<string, string> = { A1: "A2", A2: "B1", B1: "B2", B2: "C1" };
        body.from_level = englishForm.from_level; body.to_level = next[englishForm.from_level];
      }
      await api(portalPath("teacher", "assessment", "/assignments"), token, json("POST", body));
      toast.success("English service biriktirildi"); await load();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  async function createSat() {
    if (!token || !targetID) return toast.error("O‘zingizga biriktirilgan group yoki studentni tanlang");
    setBusy(true);
    try {
      await api(portalPath("teacher", "sat", "/assignments"), token, json("POST", { title: satForm.title.trim(), target_type: targetType, target_id: targetID, question_count: satForm.question_count }));
      toast.success("SAT Math service biriktirildi"); await load();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  async function createListening() {
    if (!token || !targetID || !setID) return toast.error("Listening set va targetni tanlang");
    setBusy(true);
    try {
      await api(portalPath("teacher", "listening", "/assignments"), token, json("POST", { set_id: setID, target_type: targetType, target_id: targetID }));
      toast.success("Listening service biriktirildi"); await load();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  const items = tab === "english" ? english : tab === "sat" ? sat : listening;
  return <>
    <PageHeader title="Services" subtitle="Faqat sizga biriktirilgan group va shu grouplardagi studentlarga English, SAT Math yoki Listening servislarini bering." />
    <Card className="section"><div className="row" style={{ flexWrap: "wrap" }}>
      <Button variant={tab === "english" ? "default" : "outline"} onClick={() => setTab("english")}><BookOpenCheck size={16} />English</Button>
      <Button variant={tab === "sat" ? "default" : "outline"} onClick={() => setTab("sat")}><Sigma size={16} />SAT Math</Button>
      <Button variant={tab === "listening" ? "default" : "outline"} onClick={() => setTab("listening")}><Headphones size={16} />Listening</Button>
    </div></Card>

    <Card className="section">
      <div className="grid grid-3">
        <Field label="Target type"><Select value={targetType} onChange={(e) => setTargetType(e.target.value as "group" | "student")}><option value="group">My group</option><option value="student">Student in my groups</option></Select></Field>
        <Field label="Target"><Select value={targetID} onChange={(e) => setTargetID(e.target.value)}><option value="">Select…</option>{targets.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}</Select></Field>
        {tab === "english" ? <Field label="English service"><Select value={englishForm.service_code} onChange={(e) => setEnglishForm((v) => ({ ...v, service_code: e.target.value, question_count: defaults[e.target.value] || 40 }))}>{englishServices.map((x) => <option key={x} value={x}>{x}</option>)}</Select></Field> : null}
        {tab === "english" ? <Field label="Title"><Input value={englishForm.title} onChange={(e) => setEnglishForm({ ...englishForm, title: e.target.value })} /></Field> : null}
        {tab === "english" ? <Field label="Questions"><Input type="number" min={1} max={80} value={englishForm.question_count} onChange={(e) => setEnglishForm({ ...englishForm, question_count: Number(e.target.value) })} /></Field> : null}
        {tab === "english" && englishForm.service_code === "level_upgrade" ? <Field label="From level"><Select value={englishForm.from_level} onChange={(e) => setEnglishForm({ ...englishForm, from_level: e.target.value })}>{["A1", "A2", "B1", "B2"].map((x) => <option key={x}>{x}</option>)}</Select></Field> : null}
        {tab === "sat" ? <><Field label="Title"><Input value={satForm.title} onChange={(e) => setSatForm({ ...satForm, title: e.target.value })} /></Field><Field label="Questions"><Input type="number" min={10} max={80} value={satForm.question_count} onChange={(e) => setSatForm({ ...satForm, question_count: Number(e.target.value) })} /></Field></> : null}
        {tab === "listening" ? <Field label="Listening set"><Select value={setID} onChange={(e) => setSetID(e.target.value)}><option value="">Select set…</option>{sets.map((x) => <option key={x.id} value={x.id}>{x.title}{x.level ? ` · ${x.level}` : ""}</option>)}</Select></Field> : null}
      </div>
      <Button className="accent section" disabled={busy || !targetID} onClick={() => void (tab === "english" ? createEnglish() : tab === "sat" ? createSat() : createListening())}>{busy ? "Assigning…" : "Assign service"}</Button>
    </Card>

    <div className="section">{items.length === 0 ? <Empty>Bu turdagi service assignment hali yo‘q.</Empty> : <TableWrap><Table><TableHeader><TableRow><TableHead>Service</TableHead><TableHead>Title / Set</TableHead><TableHead>Target</TableHead><TableHead>Questions</TableHead></TableRow></TableHeader><TableBody>{items.map((item) => <TableRow key={item.id}><TableCell>{item.service_code || (tab === "sat" ? "sat_math" : "listening")}</TableCell><TableCell>{item.title || item.set_id || "—"}</TableCell><TableCell>{item.target_type}</TableCell><TableCell>{item.question_count ?? "—"}</TableCell></TableRow>)}</TableBody></Table></TableWrap>}</div>
  </>;
}
