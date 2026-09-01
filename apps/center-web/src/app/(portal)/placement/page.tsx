"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Check, Copy, Download, Link2, QrCode, RefreshCw, RotateCcw, Smartphone, UserPlus } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { toast } from "sonner";
import { api, apiBlob, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Card, Empty, Field, Input, PageHeader, Pill, Progress, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap } from "@/components/ui";

type PlacementQuestion = { id: string; text: string; options: string[]; level: string; subject_code: string };
type Placement = {
  id: string;
  full_name: string;
  contact_email?: string | null;
  contact_phone?: string | null;
  mode: "digital" | "paper";
  status: "in_progress" | "completed" | "registered" | "expired";
  score?: number | null;
  level_result?: string | null;
  registered_user_id?: string | null;
  started_at: string;
  questions?: PlacementQuestion[];
  question_count: number;
  answered_count: number;
  invitation_token?: string;
  invitation_expires_at?: string | null;
  invitation_claimed_at?: string | null;
  candidate_session_expires_at?: string | null;
  candidate_last_seen_at?: string | null;
};
type PlacementListItem = Omit<Placement, "questions" | "invitation_token">;
type Breakdown = { attempts: number; correct: number; accuracy: number; points: number; max_points: number };
type FinishResult = { id: string; status: "completed"; score: number; level: string; by_level: Record<string, Breakdown> };
type Student = { user_id: string; full_name: string; email: string; current_level?: string | null; status: string };

const letters = ["A", "B", "C", "D"];
const cefr = ["A1", "A2", "B1", "B2", "C1"];

function dateTime(value?: string | null) {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString("uz-UZ", { dateStyle: "medium", timeStyle: "short" });
}

function inviteState(p: Placement) {
  if (p.status !== "in_progress" || p.mode !== "digital") return "closed";
  if (p.invitation_claimed_at && p.candidate_session_expires_at && Date.parse(p.candidate_session_expires_at) > Date.now()) return "active";
  if (p.invitation_claimed_at) return "session_expired";
  if (p.invitation_expires_at && Date.parse(p.invitation_expires_at) <= Date.now()) return "invite_expired";
  return "waiting";
}

export default function PlacementPage() {
  const auth = useAuth();
  const [setup, setSetup] = useState({ full_name: "", contact_email: "", contact_phone: "", mode: "digital" as "digital" | "paper" });
  const [placement, setPlacement] = useState<Placement | null>(null);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [result, setResult] = useState<FinishResult | null>(null);
  const [account, setAccount] = useState({ email: "", password: "" });
  const [recent, setRecent] = useState<PlacementListItem[]>([]);
  const [inviteUrl, setInviteUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);

  const loadRecent = useCallback(async () => {
    if (!auth.session) return;
    const data = await api<{ items: PlacementListItem[] }>(portalPath("center", "assessment", "/pre-registration/placements"), auth.session.access_token);
    setRecent(data.items || []);
  }, [auth.session]);

  const loadPlacement = useCallback(async (id: string) => {
    if (!auth.session) return null;
    return api<Placement>(portalPath("center", "assessment", `/pre-registration/placements/${id}`), auth.session.access_token);
  }, [auth.session]);

  useEffect(() => { void loadRecent().catch((error: Error) => toast.error(error.message)); }, [loadRecent]);

  useEffect(() => {
    if (!placement || placement.mode !== "digital" || placement.status !== "in_progress" || !auth.session) return;
    const timer = window.setInterval(() => {
      void (async () => {
        try {
          const fresh = await loadPlacement(placement.id);
          if (!fresh) return;
          setPlacement(fresh);
          if (fresh.status === "completed" && fresh.score != null && fresh.level_result) {
            setResult({ id: fresh.id, status: "completed", score: fresh.score, level: fresh.level_result, by_level: {} });
            await loadRecent();
          }
        } catch {}
      })();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [placement?.id, placement?.mode, placement?.status, auth.session, loadPlacement, loadRecent]);

  const paperQuestions = placement?.questions || [];
  const paperAnswered = useMemo(() => paperQuestions.filter((q) => Boolean(answers[q.id])).length, [paperQuestions, answers]);
  const digitalProgress = placement?.question_count ? (placement.answered_count || 0) * 100 / placement.question_count : 0;

  function buildInviteUrl(token: string) {
    if (typeof window === "undefined") return "";
    // Token stays in the URL fragment. Fragments are not sent to the web server
    // or Referer header, reducing accidental leakage in logs and analytics.
    return `${window.location.origin}/placement/invite#token=${encodeURIComponent(token)}`;
  }

  async function startPlacement() {
    if (!auth.session || busy || !setup.full_name.trim()) return;
    setBusy(true);
    try {
      const created = await api<Placement>(portalPath("center", "assessment", "/pre-registration/placements"), auth.session.access_token, json("POST", {
        full_name: setup.full_name.trim(),
        contact_email: setup.contact_email.trim(),
        contact_phone: setup.contact_phone.trim(),
        mode: setup.mode,
        question_count: 40,
      }));
      setPlacement(created); setAnswers({}); setResult(null);
      setAccount({ email: created.contact_email || "", password: "" });
      setInviteUrl(created.invitation_token ? buildInviteUrl(created.invitation_token) : "");
      toast.success(created.mode === "paper" ? "Qog‘oz placement testi tayyor" : "Xavfsiz invitation yaratildi");
      await loadRecent();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  async function resumePlacement(id: string) {
    setBusy(true);
    try {
      const loaded = await loadPlacement(id);
      if (!loaded) return;
      setPlacement(loaded); setAnswers({}); setInviteUrl("");
      setSetup({ full_name: loaded.full_name, contact_email: loaded.contact_email || "", contact_phone: loaded.contact_phone || "", mode: loaded.mode });
      setAccount({ email: loaded.contact_email || "", password: "" });
      if (loaded.status === "completed" && loaded.score != null && loaded.level_result) {
        setResult({ id: loaded.id, status: "completed", score: loaded.score, level: loaded.level_result, by_level: {} });
      } else { setResult(null); }
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  async function reissueInvitation() {
    if (!auth.session || !placement || busy) return;
    setBusy(true);
    try {
      const updated = await api<Placement>(portalPath("center", "assessment", `/pre-registration/placements/${placement.id}/invitation`), auth.session.access_token, json("POST", {}));
      setPlacement(updated);
      setInviteUrl(updated.invitation_token ? buildInviteUrl(updated.invitation_token) : "");
      toast.success("Yangi bir martalik invitation yaratildi");
      await loadRecent();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  async function copyInvite() {
    if (!inviteUrl) return;
    try {
      await navigator.clipboard.writeText(inviteUrl);
      setCopied(true); setTimeout(() => setCopied(false), 1600);
      toast.success("Invitation link nusxalandi");
    } catch { toast.error("Linkni nusxalab bo‘lmadi"); }
  }

  async function downloadPaper() {
    if (!auth.session) return;
    try {
      const blob = await apiBlob(portalPath("center", "assessment", "/pre-registration/placement-paper"), auth.session.access_token);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url; a.download = "IELTS-placement-test.docx"; document.body.appendChild(a); a.click(); a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      toast.success("Word test yuklandi");
    } catch (error: any) { toast.error(error.message); }
  }

  async function finishPaperPlacement() {
    if (!auth.session || !placement || busy) return;
    if (paperAnswered !== paperQuestions.length) { toast.error(`Barcha javoblarni kiriting (${paperAnswered}/${paperQuestions.length})`); return; }
    setBusy(true);
    try {
      const done = await api<FinishResult>(portalPath("center", "assessment", `/pre-registration/placements/${placement.id}/finish`), auth.session.access_token, json("POST", { answers }));
      setResult(done); setPlacement((p) => p ? { ...p, status: "completed", score: done.score, level_result: done.level } : p);
      toast.success(`Natija: ${done.level} • ${done.score.toFixed(0)}%`);
      await loadRecent();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  async function createAccount() {
    if (!auth.session || !placement || !result || busy) return;
    if (!account.email.trim() || account.password.length < 10) { toast.error("Email va kamida 10 belgili vaqtinchalik parol kiriting"); return; }
    setBusy(true);
    try {
      const student = await api<Student>(portalPath("center", "tenant", "/students"), auth.session.access_token, json("POST", {
        full_name: placement.full_name,
        email: account.email.trim().toLowerCase(),
        password: account.password,
        current_level: result.level,
      }));
      await api(portalPath("center", "assessment", `/pre-registration/placements/${placement.id}/registered`), auth.session.access_token, json("POST", { user_id: student.user_id }));
      setPlacement((p) => p ? { ...p, status: "registered", registered_user_id: student.user_id } : p);
      toast.success(`Student akkaunti yaratildi: ${result.level}`);
      await loadRecent();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  function resetFlow() {
    setPlacement(null); setAnswers({}); setResult(null); setAccount({ email: "", password: "" }); setInviteUrl("");
    setSetup({ full_name: "", contact_email: "", contact_phone: "", mode: "digital" });
  }

  const digitalState = placement ? inviteState(placement) : "closed";

  return <>
    <PageHeader title="Yangi student placement testi" subtitle="Digital test invitation + QR orqali nomzodning o‘z telefonida ishlaydi. Akkaunt faqat placement natijasidan keyin yaratiladi." action={placement ? <Button onClick={resetFlow}><RotateCcw size={16}/>Yangi nomzod</Button> : undefined}/>

    {!placement ? <Card className="section">
      <h3>1. Nomzod ma’lumotlari</h3>
      <p className="muted">Digital rejimda center savollarni o‘qib bermaydi va o‘z kompyuterini bermaydi. Tizim bir martalik invitation yaratadi.</p>
      <div className="grid grid-4 section">
        <Field label="F.I.Sh."><Input required autoFocus value={setup.full_name} onChange={(e) => setSetup({...setup,full_name:e.target.value})} placeholder="Ali Valiyev"/></Field>
        <Field label="Email (ixtiyoriy)"><Input type="email" value={setup.contact_email} onChange={(e) => setSetup({...setup,contact_email:e.target.value})} placeholder="student@example.com"/></Field>
        <Field label="Telefon (ixtiyoriy)"><Input value={setup.contact_phone} onChange={(e) => setSetup({...setup,contact_phone:e.target.value})} placeholder="+998 ..."/></Field>
        <Field label="Test usuli"><Select value={setup.mode} onChange={(e) => setSetup({...setup,mode:e.target.value as "digital" | "paper"})}><option value="digital">QR / invitation — student telefoni</option><option value="paper">Qog‘oz / Word</option></Select></Field>
      </div>
      <div className="row section"><Button className="accent" onClick={() => void startPlacement()} disabled={busy || !setup.full_name.trim()}>{busy ? "Tayyorlanmoqda…" : setup.mode === "digital" ? "Invitation yaratish" : "Paper placement yaratish"}</Button><span className="muted">40 savol • A1–C1 diagnostika</span></div>
    </Card> : null}

    {placement && placement.status === "in_progress" ? <div className="section stack">
      <Card>
        <div className="row-between">
          <div><b>{placement.full_name}</b><div className="muted">Placement ID: <span className="mono">{placement.id.slice(0, 12)}</span></div></div>
          <div className="row"><Pill>{placement.mode === "paper" ? "Qog‘oz" : "Digital invitation"}</Pill><Pill>{placement.mode === "paper" ? `${paperAnswered}/${paperQuestions.length}` : `${placement.answered_count || 0}/${placement.question_count}`}</Pill></div>
        </div>
        <Progress value={placement.mode === "paper" ? (paperQuestions.length ? paperAnswered * 100 / paperQuestions.length : 0) : digitalProgress} className="section"/>
      </Card>

      {placement.mode === "digital" ? <Card>
        <div className="row-between"><div><h3 style={{ marginBottom: 6 }}>2. QR-kodni studentga ko‘rsating</h3><p className="muted" style={{ margin: 0 }}>Student QR-kodni o‘z telefonida skaner qiladi. Invitation birinchi qurilmaga claim qilingach qayta ishlatilmaydi.</p></div><Smartphone size={34}/></div>

        {inviteUrl && !placement.invitation_claimed_at ? <div className="grid grid-2 section" style={{ alignItems: "center" }}>
          <div style={{ display: "grid", placeItems: "center", padding: 18, border: "1px solid var(--border)", borderRadius: 14, background: "white" }}>
            <QRCodeSVG value={inviteUrl} size={230} level="Q" title={`${placement.full_name} placement invitation`}/>
          </div>
          <div className="stack">
            <Alert><b>Xavfsizlik:</b> token QR/link ichida URL fragment sifatida turadi, server logiga yuborilmaydi. Backend faqat token xeshini saqlaydi.</Alert>
            <div className="codebox" style={{ maxHeight: 100, overflow: "auto" }}>{inviteUrl}</div>
            <div className="row"><Button className="accent" onClick={() => void copyInvite()}>{copied ? <Check size={16}/> : <Copy size={16}/>} {copied ? "Nusxalandi" : "Linkni nusxalash"}</Button><Pill>Amal qiladi: {dateTime(placement.invitation_expires_at)}</Pill></div>
          </div>
        </div> : null}

        {(!inviteUrl || !!placement.invitation_claimed_at) && digitalState === "waiting" ? <Alert className="section">Raw invitation token xavfsizlik sababli DBda saqlanmaydi. Ushbu sahifa qayta ochilgan bo‘lsa, yangi link yaratish kerak.</Alert> : null}
        {digitalState === "active" ? <Alert className="section"><b>Student testni boshladi.</b> Qurilma sessioni {dateTime(placement.candidate_session_expires_at)} gacha amal qiladi. So‘nggi faollik: {dateTime(placement.candidate_last_seen_at)}.</Alert> : null}
        {digitalState === "invite_expired" ? <Alert className="section">Invitation muddati tugagan. Yangi bir martalik link yarating.</Alert> : null}
        {digitalState === "session_expired" ? <Alert className="section">Candidate session muddati tugagan. Testni qayta boshlash uchun yangi invitation yaratish mumkin.</Alert> : null}

        <div className="row section">
          {(!inviteUrl || !!placement.invitation_claimed_at) && digitalState !== "active" ? <Button onClick={() => void reissueInvitation()} disabled={busy}><RefreshCw size={16}/>{busy ? "Yaratilmoqda…" : "Yangi invitation yaratish"}</Button> : null}
          <span className="muted"><QrCode size={15} style={{ display: "inline", verticalAlign: "-2px" }}/> Natija avtomatik ravishda shu sahifada paydo bo‘ladi.</span>
        </div>
      </Card> : <Card>
        <h3>2. Word faylni chop eting</h3>
        <p className="muted">Nomzod telefonsiz kelgan holat uchun printerga tayyor test. Test tugagach, javob varaqasidagi A/B/C/D javoblarini kiriting.</p>
        <Button className="accent" onClick={() => void downloadPaper()}><Download size={16}/>Word testni yuklab olish</Button>
        <div className="grid grid-4 section">
          {paperQuestions.map((q, i) => <Field key={q.id} label={`${i + 1}-savol`}><Select aria-label={`${i + 1}-savol javobi`} value={answers[q.id] || ""} onChange={(e) => setAnswers((a) => ({...a,[q.id]:e.target.value}))}><option value="">—</option>{q.options.map((_, j) => <option key={letters[j]} value={letters[j]}>{letters[j]}</option>)}</Select></Field>)}
        </div>
        <Button className="accent" onClick={() => void finishPaperPlacement()} disabled={busy || paperAnswered !== paperQuestions.length}>{busy ? "Hisoblanmoqda…" : "Javoblarni tekshirish va darajani aniqlash"}</Button>
      </Card>}
    </div> : null}

    {placement && result ? <Card className="section">
      <div className="row-between"><div><h2 style={{ margin: 0 }}>3. Placement natijasi</h2><p className="muted">Digital natija student telefonidan avtomatik keladi; paper natija center kiritgan javoblardan hisoblanadi.</p></div><div style={{ textAlign: "right" }}><div style={{ fontSize: 36, fontWeight: 800 }}>{result.level}</div><div className="muted">{result.score.toFixed(0)}%</div></div></div>
      {Object.keys(result.by_level).length > 0 ? <div className="grid grid-4 section">{cefr.map((level) => { const b=result.by_level[level]; return <div key={level} className="card"><b>{level}</b><div className="muted">{b ? `${b.correct}/${b.attempts} • ${b.accuracy.toFixed(0)}%` : "—"}</div></div>; })}</div> : null}
      {placement.status !== "registered" ? <div className="section"><h3>4. Student akkauntini yarating</h3><Alert>Daraja qo‘lda tanlanmaydi: yangi akkaunt <b>{result.level}</b> daraja bilan yaratiladi.</Alert><div className="grid grid-3 section"><Field label="F.I.Sh."><Input value={placement.full_name} disabled/></Field><Field label="Login email"><Input type="email" value={account.email} onChange={(e) => setAccount({...account,email:e.target.value})}/></Field><Field label="Vaqtinchalik parol"><Input type="password" autoComplete="new-password" minLength={10} value={account.password} onChange={(e) => setAccount({...account,password:e.target.value})} placeholder="kamida 10 belgi"/></Field></div><Button className="accent" onClick={() => void createAccount()} disabled={busy || !account.email.trim() || account.password.length < 10}><UserPlus size={16}/>{busy ? "Yaratilmoqda…" : `${result.level} darajada student yaratish`}</Button></div> : <Alert className="section"><b>Ro‘yxatdan o‘tkazildi.</b> Student endi student portaliga login orqali kiradi.</Alert>}
    </Card> : null}

    <div className="section"><h2>So‘nggi placementlar</h2>{recent.length === 0 ? <Empty>Hali placement testi yaratilmagan.</Empty> : <TableWrap><Table><TableHeader><TableRow><TableHead>Nomzod</TableHead><TableHead>Usul</TableHead><TableHead>Jarayon</TableHead><TableHead>Natija</TableHead><TableHead></TableHead></TableRow></TableHeader><TableBody>{recent.slice(0,20).map((item) => <TableRow key={item.id}><TableCell><b>{item.full_name}</b><div className="muted">{item.contact_email || item.contact_phone || "Kontakt kiritilmagan"}</div></TableCell><TableCell>{item.mode === "paper" ? "Qog‘oz" : <span><Link2 size={14} style={{ display: "inline", verticalAlign: "-2px" }}/> QR invitation</span>}</TableCell><TableCell><Pill tone={item.status === "registered" ? "ok" : ""}>{item.status}</Pill><div className="muted" style={{ marginTop: 4 }}>{item.mode === "digital" && item.status === "in_progress" ? `${item.answered_count || 0}/${item.question_count || 40} javob` : ""}</div></TableCell><TableCell>{item.level_result ? <b>{item.level_result} • {item.score?.toFixed(0)}%</b> : "—"}</TableCell><TableCell>{item.status !== "registered" ? <Button size="sm" onClick={() => void resumePlacement(item.id)}>Ochish</Button> : null}</TableCell></TableRow>)}</TableBody></Table></TableWrap>}</div>
  </>;
}
