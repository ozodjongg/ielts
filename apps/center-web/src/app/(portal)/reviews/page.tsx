"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { api, apiBlob, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Card, Empty, Field, Input, PageHeader, Pill, Textarea } from "@/components/ui";

type Submission = { id: string; student_user_id: string; service_code: string; prompt_id: string; status: string; text_submission?: string; has_audio: boolean; submitted_at: string };
type Criterion = { key: string; label: string; hint: string };

const writingCriteria: Criterion[] = [
  { key: "task_response", label: "Task response", hint: "Topshiriqni to‘liq va aniq bajarishi" },
  { key: "coherence", label: "Coherence & cohesion", hint: "Fikrlar ketma-ketligi va bog‘lanishi" },
  { key: "lexical_resource", label: "Lexical resource", hint: "Lug‘at boyligi va so‘z tanlovi" },
  { key: "grammar", label: "Grammar", hint: "Grammatik diapazon va aniqlik" },
];
const speakingCriteria: Criterion[] = [
  { key: "fluency", label: "Fluency & coherence", hint: "Ravonlik va fikrni bog‘lab yetkazish" },
  { key: "pronunciation", label: "Pronunciation", hint: "Talaffuzning tushunarliligi" },
  { key: "vocabulary", label: "Vocabulary", hint: "Lug‘at diapazoni va aniqligi" },
  { key: "grammar", label: "Grammar", hint: "Grammatik diapazon va aniqlik" },
];

function blankScores(criteria: Criterion[]) { return Object.fromEntries(criteria.map((c) => [c.key, 20])) as Record<string, number>; }

export default function ReviewsPage() {
  const auth = useAuth();
  const [items, setItems] = useState<Submission[]>([]);
  const [selected, setSelected] = useState<Submission | null>(null);
  const [notes, setNotes] = useState("");
  const criteria = selected?.service_code === "speaking" ? speakingCriteria : writingCriteria;
  const [scores, setScores] = useState<Record<string, number>>(() => blankScores(writingCriteria));
  const score = useMemo(() => criteria.reduce((sum, c) => sum + Number(scores[c.key] || 0), 0), [criteria, scores]);

  const load = useCallback(async () => {
    if (!auth.session) return;
    const result = await api<{ items: Submission[] }>(portalPath("center", "review", "/submissions?status=pending"), auth.session.access_token);
    setItems(result.items || []);
  }, [auth.session]);
  useEffect(() => { void load().catch((error: Error) => toast.error(error.message)); }, [load]);

  function choose(item: Submission) {
    const nextCriteria = item.service_code === "speaking" ? speakingCriteria : writingCriteria;
    setSelected(item); setNotes(""); setScores(blankScores(nextCriteria));
  }

  async function completeReview() {
    if (!auth.session || !selected) return;
    if (score < 0 || score > 100 || criteria.some((c) => scores[c.key] < 0 || scores[c.key] > 25)) { toast.error("Har bir mezon 0–25 oralig‘ida bo‘lishi kerak"); return; }
    try {
      await api(portalPath("center", "review", `/submissions/${selected.id}/review`), auth.session.access_token, json("PATCH", { score, notes, rubric: scores }));
      toast.success("Baholash saqlandi"); setSelected(null); setNotes(""); await load();
    } catch (error: any) { toast.error(error.message); }
  }

  return <><PageHeader title="Speaking & Writing review" subtitle="Dasturchilar uchun JSON emas — ustoz uchun tushunarli 4 ta baholash mezoni."/><div className="grid grid-2 section"><div className="stack">{items.length === 0 ? <Empty>Tekshirilishi kerak bo‘lgan ish yo‘q.</Empty> : items.map((item) => <Card key={item.id}><div className="row-between"><div><b>{item.service_code === "speaking" ? "Speaking" : "Writing"}</b><div className="muted mono">{item.student_user_id.slice(0, 12)}</div></div><Pill>{item.status}</Pill></div><p className="muted">Prompt: {item.prompt_id}</p>{item.text_submission && <p style={{ whiteSpace: "pre-wrap" }}>{item.text_submission.slice(0, 700)}</p>}<Button onClick={() => choose(item)}>Baholash</Button></Card>)}</div>{selected ? <Card><div className="row-between"><div><h3 style={{margin:0}}>Ishni baholash</h3><p className="muted">{selected.prompt_id}</p></div><div style={{fontSize:32,fontWeight:800}}>{score}/100</div></div>{selected.has_audio && auth.session && <AudioPlayer id={selected.id} token={auth.session.access_token}/>}<Alert className="section">Har bir mezonni <b>0 dan 25 gacha</b> baholang. Umumiy ball avtomatik hisoblanadi.</Alert><div className="grid grid-2 section">{criteria.map((criterion) => <Field key={criterion.key} label={`${criterion.label} (0–25)`} hint={criterion.hint}><Input type="number" min={0} max={25} step={1} value={scores[criterion.key] ?? 0} onChange={(e) => setScores((current) => ({...current,[criterion.key]:Math.max(0,Math.min(25,Number(e.target.value))) }))}/></Field>)}</div><Field label="Ustoz izohi" hint="Studentga foydali bo‘ladigan kuchli va yaxshilanishi kerak bo‘lgan tomonlarni yozing."><Textarea value={notes} onChange={(e) => setNotes(e.target.value)} placeholder="Masalan: fikrlar aniq, lekin murakkab gaplarda grammatik xatolar bor..."/></Field><Button className="accent section" onClick={() => void completeReview()} disabled={score < 0 || score > 100}>Baholashni yakunlash • {score}/100</Button></Card> : <Card><div className="muted">Chap tomondan tekshiriladigan ishni tanlang.</div></Card>}</div></>;
}

function AudioPlayer({ id, token }: { id: string; token: string }) {
  const [url, setURL] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true; let objectURL = "";
    void apiBlob(portalPath("center", "review", `/submissions/${id}/audio`), token)
      .then((blob) => { if (!active) return; objectURL = URL.createObjectURL(blob); setURL(objectURL); setError(""); })
      .catch((e: Error) => { if (active) setError(e.message); });
    return () => { active = false; if (objectURL) URL.revokeObjectURL(objectURL); };
  }, [id, token]);
  if (error) return <div className="error">{error}</div>;
  return url ? <audio controls preload="metadata" src={url} style={{ width: "100%" }} /> : <div className="muted">Audio yuklanmoqda…</div>;
}
