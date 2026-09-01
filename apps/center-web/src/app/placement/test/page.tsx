"use client";

import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, ShieldCheck } from "lucide-react";
import { api, json } from "@/lib/api";
import { Alert, Button, Card, Loading, Pill, Progress } from "@/components/ui";

const SESSION_KEY = "ielts-placement-candidate-session";
const letters = ["A", "B", "C", "D"];

type PlacementQuestion = { id: string; text: string; options: string[] };
type CandidateSession = {
  id: string;
  full_name: string;
  status: "in_progress";
  question_count: number;
  answered_count: number;
  questions: PlacementQuestion[];
  answers: Record<string, string>;
  session_expires_at: string;
};
type SavedSession = { token: string; placement_id: string; expires_at: string };
type FinishResponse = { id: string; status: "completed"; score: number; level: string };

export default function CandidatePlacementTestPage() {
  const [saved, setSaved] = useState<SavedSession | null>(null);
  const [placement, setPlacement] = useState<CandidateSession | null>(null);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [index, setIndex] = useState(0);
  const [busyQuestion, setBusyQuestion] = useState("");
  const [finishing, setFinishing] = useState(false);
  const [result, setResult] = useState<FinishResponse | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const raw = localStorage.getItem(SESSION_KEY);
        if (!raw) throw new Error("Placement session topilmadi. Center bergan QR-kodni skaner qiling.");
        const local = JSON.parse(raw) as SavedSession;
        if (!local?.token || Date.parse(local.expires_at) <= Date.now()) {
          localStorage.removeItem(SESSION_KEY);
          throw new Error("Placement session muddati tugagan. Centerdan yangi invitation oling.");
        }
        const data = await api<CandidateSession>("/public/placement/session", "", { headers: { "X-Placement-Session": local.token } });
        if (!active) return;
        setSaved(local);
        setPlacement(data);
        setAnswers(data.answers || {});
        const firstUnanswered = data.questions.findIndex((q) => !data.answers?.[q.id]);
        setIndex(firstUnanswered >= 0 ? firstUnanswered : Math.max(0, data.questions.length - 1));
      } catch (e: any) {
        if (active) setError(e?.message || "Testni ochib bo‘lmadi.");
      }
    })();
    return () => { active = false; };
  }, []);

  const answered = useMemo(() => placement ? placement.questions.filter((q) => Boolean(answers[q.id])).length : 0, [placement, answers]);
  const current = placement?.questions[index];

  async function chooseAnswer(question: PlacementQuestion, answer: string) {
    if (!saved || busyQuestion) return;
    const previous = answers[question.id];
    setAnswers((a) => ({ ...a, [question.id]: answer }));
    setBusyQuestion(question.id);
    try {
      await api("/public/placement/session/answer", "", {
        ...json("POST", { question_id: question.id, answer }),
        headers: { "X-Placement-Session": saved.token },
      });
    } catch (e: any) {
      setAnswers((a) => ({ ...a, [question.id]: previous || "" }));
      setError(e?.message || "Javobni saqlab bo‘lmadi.");
    } finally {
      setBusyQuestion("");
    }
  }

  async function finish() {
    if (!saved || !placement || answered !== placement.questions.length || finishing) return;
    setFinishing(true); setError("");
    try {
      const done = await api<FinishResponse>("/public/placement/session/finish", "", {
        ...json("POST", {}),
        headers: { "X-Placement-Session": saved.token },
      });
      localStorage.removeItem(SESSION_KEY);
      setResult(done);
    } catch (e: any) {
      setError(e?.message || "Testni yakunlab bo‘lmadi.");
    } finally { setFinishing(false); }
  }

  if (error && !placement) return <main className="login-wrap"><Card className="login-card"><Alert>{error}</Alert><p className="muted section">Center bergan QR-kodni qayta skaner qiling yoki yangi invitation so‘rang.</p></Card></main>;
  if (!placement) return <main className="login-wrap"><Card className="login-card"><Loading label="Placement test yuklanmoqda…"/></Card></main>;
  if (result) return <main className="login-wrap"><Card className="login-card" style={{ textAlign: "center" }}><div style={{ display: "grid", placeItems: "center", gap: 12 }}><CheckCircle2 size={48}/><h1 style={{ margin: 0 }}>Test yakunlandi</h1><div style={{ fontSize: 48, fontWeight: 800 }}>{result.level}</div><div className="muted">Natija: {result.score.toFixed(0)}%</div><Alert>Natija centerga yuborildi. Student akkauntini center xodimi yaratadi.</Alert></div></Card></main>;

  return <main style={{ minHeight: "100vh", background: "var(--surface)", padding: "max(16px, env(safe-area-inset-top)) 12px max(28px, env(safe-area-inset-bottom))" }}>
    <div style={{ width: "min(760px, 100%)", margin: "0 auto" }}>
      <Card>
        <div className="row-between">
          <div><div className="row"><ShieldCheck size={18}/><b>IELTS Placement Test</b></div><div className="muted" style={{ marginTop: 6 }}>{placement.full_name}</div></div>
          <Pill>{answered}/{placement.questions.length}</Pill>
        </div>
        <Progress value={placement.questions.length ? answered * 100 / placement.questions.length : 0} className="section"/>
      </Card>

      {error ? <Alert className="section">{error}</Alert> : null}
      {current ? <Card className="section">
        <div className="row-between"><Pill>Savol {index + 1}/{placement.questions.length}</Pill><span className="muted">Javob avtomatik saqlanadi</span></div>
        <h2 style={{ marginTop: 22, lineHeight: 1.45, fontSize: "clamp(19px,4.5vw,26px)" }}>{current.text}</h2>
        <div className="stack section">
          {current.options.map((option, j) => <Button
            key={letters[j]}
            variant={answers[current.id] === letters[j] ? "default" : "outline"}
            disabled={busyQuestion === current.id}
            style={{ justifyContent: "flex-start", minHeight: 52, height: "auto", whiteSpace: "normal", textAlign: "left", paddingBlock: 12 }}
            onClick={() => void chooseAnswer(current, letters[j])}
          ><b>{letters[j]}.</b> {option}</Button>)}
        </div>
        <div className="row-between section">
          <Button disabled={index === 0 || Boolean(busyQuestion)} onClick={() => setIndex((v) => Math.max(0, v - 1))}>Orqaga</Button>
          {index < placement.questions.length - 1 ? <Button variant="default" disabled={!answers[current.id] || Boolean(busyQuestion)} onClick={() => setIndex((v) => Math.min(placement.questions.length - 1, v + 1))}>Keyingi</Button> : <Button variant="default" disabled={answered !== placement.questions.length || finishing || Boolean(busyQuestion)} onClick={() => void finish()}>{finishing ? "Hisoblanmoqda…" : "Testni yakunlash"}</Button>}
        </div>
      </Card> : null}
    </div>
  </main>;
}
