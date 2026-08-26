"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { api, apiBlob, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, Field, Input, PageHeader, Pill, Textarea } from "@/components/ui";

type Submission = { id: string; student_user_id: string; service_code: string; prompt_id: string; status: string; text_submission?: string; has_audio: boolean; submitted_at: string };

export default function ReviewsPage() {
  const auth = useAuth();
  const [items, setItems] = useState<Submission[]>([]);
  const [selected, setSelected] = useState<Submission | null>(null);
  const [score, setScore] = useState(75);
  const [notes, setNotes] = useState("");
  const [rubric, setRubric] = useState('{"task":25,"accuracy":25,"range":25,"coherence":25}');

  const load = useCallback(async () => {
    if (!auth.session) return;
    const result = await api<{ items: Submission[] }>(portalPath("center", "review", "/submissions?status=pending"), auth.session.access_token);
    setItems(result.items || []);
  }, [auth.session]);
  useEffect(() => { void load().catch((error: Error) => toast.error(error.message)); }, [load]);

  async function completeReview() {
    if (!auth.session || !selected) return;
    try {
      const parsedRubric = JSON.parse(rubric);
      await api(portalPath("center", "review", `/submissions/${selected.id}/review`), auth.session.access_token, json("PATCH", { score, notes, rubric: parsedRubric }));
      toast.success("Review saved"); setSelected(null); setNotes(""); await load();
    } catch (error: any) { toast.error(error instanceof SyntaxError ? "Rubric JSON is invalid" : error.message); }
  }

  return <><PageHeader title="Speaking & Writing review" subtitle="Manual rubric queue; automated scores are preserved separately."/><div className="grid grid-2 section"><div className="stack">{items.length === 0 ? <Empty>No pending submissions.</Empty> : items.map((item) => <Card key={item.id}><div className="row-between"><div><b>{item.service_code}</b><div className="muted mono">{item.student_user_id.slice(0, 12)}</div></div><Pill>{item.status}</Pill></div><p className="muted">Prompt: {item.prompt_id}</p>{item.text_submission && <p style={{ whiteSpace: "pre-wrap" }}>{item.text_submission.slice(0, 600)}</p>}<Button onClick={() => { setSelected(item); setNotes(""); }}>Review</Button></Card>)}</div>{selected ? <Card><h3>Review submission</h3><p className="muted">{selected.prompt_id}</p>{selected.has_audio && auth.session && <AudioPlayer id={selected.id} token={auth.session.access_token}/>}<div className="stack section"><Field label="Score 0–100"><Input type="number" min={0} max={100} value={score} onChange={(e) => setScore(Number(e.target.value))}/></Field><Field label="Rubric JSON"><Textarea value={rubric} onChange={(e) => setRubric(e.target.value)}/></Field><Field label="Reviewer notes"><Textarea value={notes} onChange={(e) => setNotes(e.target.value)}/></Field><Button className="accent" onClick={() => void completeReview()} disabled={score < 0 || score > 100}>Complete review</Button></div></Card> : <Card><div className="muted">Select a pending submission.</div></Card>}</div></>;
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
  return url ? <audio controls preload="metadata" src={url} style={{ width: "100%" }} /> : <div className="muted">Audio loading…</div>;
}
