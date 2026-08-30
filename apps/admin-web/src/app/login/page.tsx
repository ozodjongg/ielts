"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ShieldCheck } from "lucide-react";
import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Field, Input } from "@/components/ui";

export default function LoginPage() {
  const auth = useAuth();
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => { if (auth.profile) router.replace("/"); }, [auth.profile, router]);

  async function submitPassword(event: React.FormEvent) {
    event.preventDefault(); if (busy) return; setBusy(true); setError("");
    try { const result = await auth.login(email.trim(), password); if (result === "authenticated") router.replace("/"); }
    catch (cause: unknown) { setError(cause instanceof Error ? cause.message : "Login xatosi"); }
    finally { setBusy(false); }
  }

  async function submitMfa(event: React.FormEvent) {
    event.preventDefault(); if (busy) return; setBusy(true); setError("");
    try { await auth.verifyMfa(code); router.replace("/"); }
    catch (cause: unknown) { setError(cause instanceof Error ? cause.message : "Tasdiqlash kodi noto‘g‘ri"); }
    finally { setBusy(false); }
  }

  return (
    <div className="login-wrap">
      <div className="login-card">
        <div className="brand"><span className="brandmark">IELTS</span><span>IELTS Admin</span></div>
        {!auth.mfaChallenge ? (<>
          <h1 className="title">Kirish</h1><p className="subtitle">SaaS operator portal</p>
          <form className="stack section" onSubmit={submitPassword}>
            <Field label="Email"><Input type="email" inputMode="email" autoComplete="email" required maxLength={254} value={email} onChange={(e)=>setEmail(e.target.value)} /></Field>
            <Field label="Parol"><Input type="password" autoComplete="current-password" required minLength={10} maxLength={128} value={password} onChange={(e)=>setPassword(e.target.value)} /></Field>
            {error || auth.error ? <Alert>{error || auth.error}</Alert> : null}
            <Button type="submit" disabled={busy || auth.loading}>{busy ? "Tekshirilmoqda…" : "Portalga kirish"}</Button>
          </form>
        </>) : (<>
          <div className="row" style={{marginTop:8}}><ShieldCheck size={22}/><h1 className="title" style={{fontSize:24}}>Ikki bosqichli tasdiqlash</h1></div>
          <p className="subtitle">Authenticator ilovangizdagi 6 xonali kodni yoki recovery code’ni kiriting.</p>
          <form className="stack section" onSubmit={submitMfa}>
            <Field label="TOTP / recovery code"><Input inputMode="text" autoCapitalize="characters" autoComplete="one-time-code" required value={code} onChange={(e)=>setCode(e.target.value)} placeholder="123456" /></Field>
            {error || auth.error ? <Alert>{error || auth.error}</Alert> : null}
            <Button type="submit" disabled={busy || auth.loading}>{busy ? "Tasdiqlanmoqda…" : "Tasdiqlash"}</Button>
            <Button type="button" variant="outline" onClick={()=>{auth.cancelMfa();setCode("");}}>Orqaga</Button>
          </form>
        </>)}
        <div className="divider" /><p className="muted" style={{fontSize:12}}>Authentication IELTS backend tomonidan boshqariladi. Parol, TOTP/AAL2, sessiya, rol va ruxsatlar server-side tekshiriladi.</p>
      </div>
    </div>
  );
}
