"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Plus, Trash2, Upload } from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "@/components/auth-provider";
import { api, json, portalPath } from "@/lib/api";
import { Button, Card, Empty, Field, Input, PageHeader, Pill, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap, Textarea } from "@/components/ui";

type Audio = { id: string; title: string; level?: string | null; size_bytes: number; max_plays: number; allow_seek: boolean; status: string };
type SetItem = { id: string; title: string; audio_id: string; level?: string | null; questions?: unknown[] };
type Assignment = { id: string; set_id: string; target_type: string; target_id?: string | null; due_at?: string | null; created_at: string };
type Group = { id: string; name: string };
type Student = { user_id: string; full_name: string };
type DraftQuestion = { id: string; prompt: string; options: string[]; answerIndex: number; basePoints: number };

function newQuestion(index: number): DraftQuestion {
  return { id: `q${index}`, prompt: "", options: ["", "", "", ""], answerIndex: 0, basePoints: 2 };
}

export default function ListeningPage() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [audio, setAudio] = useState<Audio[]>([]);
  const [sets, setSets] = useState<SetItem[]>([]);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [busy, setBusy] = useState(false);

  const [upload, setUpload] = useState({ title: "", level: "B1", max_plays: 2, allow_seek: false });
  const [audioFile, setAudioFile] = useState<File | null>(null);
  const [setForm, setSetForm] = useState({ title: "", audio_id: "", level: "B1" });
  const [questions, setQuestions] = useState<DraftQuestion[]>([newQuestion(1)]);
  const [assignment, setAssignment] = useState({ set_id: "", target_type: "all", target_id: "", due_at: "" });

  const load = useCallback(async () => {
    if (!token) return;
    const [a, s, asg, g, st] = await Promise.all([
      api<{ items: Audio[] }>(portalPath("center", "listening", "/audio"), token),
      api<{ items: SetItem[] }>(portalPath("center", "listening", "/sets"), token),
      api<{ items: Assignment[] }>(portalPath("center", "listening", "/assignments"), token),
      api<{ items: Group[] }>(portalPath("center", "tenant", "/groups"), token),
      api<{ items: Student[] }>(portalPath("center", "tenant", "/students"), token),
    ]);
    setAudio(a.items || []); setSets(s.items || []); setAssignments(asg.items || []); setGroups(g.items || []); setStudents(st.items || []);
  }, [token]);

  useEffect(() => { void load().catch((error: Error) => toast.error(error.message)); }, [load]);
  const targets = useMemo(() => assignment.target_type === "group" ? groups.map((x) => ({ id: x.id, label: x.name })) : students.map((x) => ({ id: x.user_id, label: x.full_name })), [assignment.target_type, groups, students]);

  async function uploadAudio() {
    if (!token || !audioFile || !upload.title.trim()) return toast.error("Audio file va title required");
    setBusy(true);
    try {
      const body = new FormData();
      body.set("audio", audioFile); body.set("title", upload.title.trim()); body.set("level", upload.level); body.set("max_plays", String(upload.max_plays)); body.set("allow_seek", String(upload.allow_seek));
      await api(portalPath("center", "listening", "/audio"), token, { method: "POST", body });
      toast.success("Audio uploaded"); setAudioFile(null); setUpload({ title: "", level: "B1", max_plays: 2, allow_seek: false }); await load();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  function updateQuestion(index: number, patch: Partial<DraftQuestion>) {
    setQuestions((current) => current.map((item, i) => i === index ? { ...item, ...patch } : item));
  }
  function updateOption(questionIndex: number, optionIndex: number, value: string) {
    setQuestions((current) => current.map((item, i) => i === questionIndex ? { ...item, options: item.options.map((option, j) => j === optionIndex ? value : option) } : item));
  }
  function addOption(questionIndex: number) {
    setQuestions((current) => current.map((item, i) => i === questionIndex && item.options.length < 6 ? { ...item, options: [...item.options, ""] } : item));
  }
  function removeOption(questionIndex: number, optionIndex: number) {
    setQuestions((current) => current.map((item, i) => {
      if (i !== questionIndex || item.options.length <= 2) return item;
      const options = item.options.filter((_, j) => j !== optionIndex);
      const answerIndex = item.answerIndex === optionIndex ? 0 : item.answerIndex > optionIndex ? item.answerIndex - 1 : item.answerIndex;
      return { ...item, options, answerIndex };
    }));
  }

  async function createSet() {
    if (!token || !setForm.audio_id || !setForm.title.trim()) return toast.error("Audio va set title required");
    const normalized = questions.map((question, index) => ({ ...question, id: `q${index + 1}`, prompt: question.prompt.trim(), options: question.options.map((x) => x.trim()) }));
    for (const question of normalized) {
      if (!question.prompt || question.options.length < 2 || question.options.some((x) => !x)) return toast.error("Har bir savol va barcha variantlarni to‘ldiring");
      if (new Set(question.options).size !== question.options.length) return toast.error("Bir savolda variantlar takrorlanmasin");
    }
    const payloadQuestions = normalized.map((question) => ({ id: question.id, prompt: question.prompt, options: question.options, base_points: question.basePoints }));
    const answerKey = Object.fromEntries(normalized.map((question) => [question.id, question.options[question.answerIndex]]));
    setBusy(true);
    try {
      await api(portalPath("center", "listening", "/sets"), token, json("POST", { audio_id: setForm.audio_id, title: setForm.title.trim(), level: setForm.level, questions: payloadQuestions, answer_key: answerKey }));
      toast.success("Listening set created"); setSetForm({ title: "", audio_id: "", level: "B1" }); setQuestions([newQuestion(1)]); await load();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  async function createAssignment() {
    if (!token || !assignment.set_id) return toast.error("Listening setni tanlang");
    if (assignment.target_type !== "all" && !assignment.target_id) return toast.error("Targetni tanlang");
    const body: Record<string, unknown> = { set_id: assignment.set_id, target_type: assignment.target_type };
    if (assignment.target_type !== "all") body.target_id = assignment.target_id;
    if (assignment.due_at) body.due_at = new Date(assignment.due_at).toISOString();
    setBusy(true);
    try {
      await api(portalPath("center", "listening", "/assignments"), token, json("POST", body));
      toast.success("Listening assigned"); setAssignment({ set_id: "", target_type: "all", target_id: "", due_at: "" }); await load();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  return <>
    <PageHeader title="Listening" subtitle="Audio yuklang, savollarni oddiy forma orqali tuzing va student/groupga biriktiring. JSON yozish talab qilinmaydi." />

    <Card className="section">
      <h3>1. Audio upload</h3>
      <div className="grid grid-3">
        <Field label="Title"><Input value={upload.title} onChange={(e) => setUpload({ ...upload, title: e.target.value })} placeholder="Unit 4 conversation" /></Field>
        <Field label="Level"><Select value={upload.level} onChange={(e) => setUpload({ ...upload, level: e.target.value })}>{["A1", "A2", "B1", "B2", "C1", "C2"].map((x) => <option key={x}>{x}</option>)}</Select></Field>
        <Field label="Maximum plays"><Input type="number" min={1} max={10} value={upload.max_plays} onChange={(e) => setUpload({ ...upload, max_plays: Number(e.target.value) })} /></Field>
        <Field label="Audio file"><Input type="file" accept="audio/webm,audio/ogg,audio/mpeg,audio/mp4,audio/wav,audio/x-wav,.mp3,.wav,.ogg,.m4a" onChange={(e) => setAudioFile(e.target.files?.[0] || null)} /></Field>
        <label className="row" style={{ alignSelf: "end", minHeight: 42 }}><input type="checkbox" checked={upload.allow_seek} onChange={(e) => setUpload({ ...upload, allow_seek: e.target.checked })} /> Allow seeking</label>
      </div>
      <Button className="section" onClick={() => void uploadAudio()} disabled={busy || !audioFile || !upload.title.trim()}><Upload size={16} />Upload audio</Button>
      {audio.length ? <div className="row section" style={{ flexWrap: "wrap" }}>{audio.slice(0, 8).map((item) => <Pill key={item.id}>{item.title} · {item.level || "—"}</Pill>)}</div> : null}
    </Card>

    <Card className="section">
      <h3>2. Build listening set</h3>
      <div className="grid grid-3">
        <Field label="Set title"><Input value={setForm.title} onChange={(e) => setSetForm({ ...setForm, title: e.target.value })} placeholder="Unit 4 comprehension" /></Field>
        <Field label="Audio"><Select value={setForm.audio_id} onChange={(e) => setSetForm({ ...setForm, audio_id: e.target.value })}><option value="">Select uploaded audio…</option>{audio.map((item) => <option key={item.id} value={item.id}>{item.title}</option>)}</Select></Field>
        <Field label="Level"><Select value={setForm.level} onChange={(e) => setSetForm({ ...setForm, level: e.target.value })}>{["A1", "A2", "B1", "B2", "C1", "C2"].map((x) => <option key={x}>{x}</option>)}</Select></Field>
      </div>

      <div className="stack section">{questions.map((question, questionIndex) => <Card key={`${question.id}-${questionIndex}`}>
        <div className="row-between"><h4>Question {questionIndex + 1}</h4>{questions.length > 1 ? <Button variant="destructive" onClick={() => setQuestions((current) => current.filter((_, i) => i !== questionIndex))}><Trash2 size={15} />Remove</Button> : null}</div>
        <Field label="Question"><Textarea value={question.prompt} onChange={(e) => updateQuestion(questionIndex, { prompt: e.target.value })} placeholder="What is the speaker's main reason?" /></Field>
        <div className="stack section">{question.options.map((option, optionIndex) => <div className="row" key={optionIndex} style={{ alignItems: "center" }}>
          <input type="radio" name={`correct-${questionIndex}`} aria-label={`Correct answer ${optionIndex + 1}`} checked={question.answerIndex === optionIndex} onChange={() => updateQuestion(questionIndex, { answerIndex: optionIndex })} />
          <Input value={option} onChange={(e) => updateOption(questionIndex, optionIndex, e.target.value)} placeholder={`Option ${String.fromCharCode(65 + optionIndex)}`} />
          {question.options.length > 2 ? <Button variant="ghost" onClick={() => removeOption(questionIndex, optionIndex)}><Trash2 size={15} /></Button> : null}
        </div>)}</div>
        <div className="row"><Button variant="outline" disabled={question.options.length >= 6} onClick={() => addOption(questionIndex)}><Plus size={15} />Add option</Button><Field label="Points"><Input type="number" min={0.5} max={20} step={0.5} value={question.basePoints} onChange={(e) => updateQuestion(questionIndex, { basePoints: Number(e.target.value) })} /></Field></div>
        <div className="muted">Correct answerni variant yonidagi radio tugma bilan belgilang.</div>
      </Card>)}</div>
      <div className="row section"><Button variant="outline" disabled={questions.length >= 50} onClick={() => setQuestions((current) => [...current, newQuestion(current.length + 1)])}><Plus size={16} />Add question</Button><Button className="accent" disabled={busy || !setForm.audio_id || !setForm.title.trim()} onClick={() => void createSet()}>Create listening set</Button></div>
    </Card>

    <Card className="section">
      <h3>3. Assign to students</h3>
      <div className="grid grid-3">
        <Field label="Listening set"><Select value={assignment.set_id} onChange={(e) => setAssignment({ ...assignment, set_id: e.target.value })}><option value="">Select set…</option>{sets.map((item) => <option key={item.id} value={item.id}>{item.title}{item.level ? ` · ${item.level}` : ""}</option>)}</Select></Field>
        <Field label="Target"><Select value={assignment.target_type} onChange={(e) => setAssignment({ ...assignment, target_type: e.target.value, target_id: "" })}><option value="all">All students</option><option value="group">Group</option><option value="student">Student</option></Select></Field>
        {assignment.target_type !== "all" ? <Field label="Target item"><Select value={assignment.target_id} onChange={(e) => setAssignment({ ...assignment, target_id: e.target.value })}><option value="">Select…</option>{targets.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}</Select></Field> : null}
        <Field label="Due date/time"><Input type="datetime-local" value={assignment.due_at} onChange={(e) => setAssignment({ ...assignment, due_at: e.target.value })} /></Field>
      </div>
      <Button className="accent section" disabled={busy || !assignment.set_id} onClick={() => void createAssignment()}>Assign listening</Button>
    </Card>

    <div className="section">{assignments.length === 0 ? <Empty>No listening assignments yet.</Empty> : <TableWrap><Table><TableHeader><TableRow><TableHead>Set</TableHead><TableHead>Target</TableHead><TableHead>Due</TableHead><TableHead>Created</TableHead></TableRow></TableHeader><TableBody>{assignments.map((item) => <TableRow key={item.id}><TableCell>{sets.find((set) => set.id === item.set_id)?.title || item.set_id}</TableCell><TableCell>{item.target_type}</TableCell><TableCell>{item.due_at ? new Date(item.due_at).toLocaleString() : "—"}</TableCell><TableCell>{new Date(item.created_at).toLocaleDateString()}</TableCell></TableRow>)}</TableBody></Table></TableWrap>}</div>
  </>;
}
