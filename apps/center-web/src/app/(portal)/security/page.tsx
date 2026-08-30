"use client";

import { useEffect, useState } from "react";
import { Copy, KeyRound, ShieldCheck, ShieldOff } from "lucide-react";
import { toast } from "sonner";
import { MfaQr } from "@/components/mfa-qr";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Card, Field, Input, PageHeader, Pill } from "@/components/ui";

type PrivilegedRole = "admin" | "center" | "teacher";
type Status = { enabled: boolean; aal: string; verified_at?: string | null };
type Setup = { secret: string; otpauth_uri: string; issuer: string; account: string };
type VerifyResult = { enabled: boolean; aal: string; recovery_codes: string[] };

export default function SecurityPage() {
  const auth = useAuth();
  const [status, setStatus] = useState<Status | null>(null);
  const [setup, setSetup] = useState<Setup | null>(null);
  const [code, setCode] = useState("");
  const [recovery, setRecovery] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const portal = auth.profile?.role as PrivilegedRole | undefined;

  async function load() {
    if (!auth.session || !portal || !["admin", "center", "teacher"].includes(portal)) return;
    try {
      setStatus(await api<Status>(portalPath(portal, "identity", "/mfa/status"), auth.session.access_token));
    } catch (e: any) {
      toast.error(e.message);
    }
  }

  useEffect(() => {
    void load();
  }, [auth.session?.access_token, portal]);

  async function start() {
    if (!auth.session || !portal) return;
    setBusy(true);
    try {
      setSetup(await api<Setup>(portalPath(portal, "identity", "/mfa/setup"), auth.session.access_token, json("POST", {})));
      setRecovery([]);
      toast.success("Authenticator setup tayyor");
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      setBusy(false);
    }
  }

  async function verify() {
    if (!auth.session || !setup || !portal) return;
    setBusy(true);
    try {
      const result = await api<VerifyResult>(portalPath(portal, "identity", "/mfa/verify"), auth.session.access_token, json("POST", { code }));
      setRecovery(result.recovery_codes || []);
      setSetup(null);
      setCode("");
      await auth.refresh();
      await load();
      toast.success("MFA yoqildi");
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      setBusy(false);
    }
  }

  async function disable() {
    if (!auth.session || !portal) return;
    setBusy(true);
    try {
      await api(portalPath(portal, "identity", "/mfa/disable"), auth.session.access_token, json("POST", { code }));
      toast.success("MFA o‘chirildi. Qayta login qiling.");
      await auth.logout();
      location.href = "/login";
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      setBusy(false);
    }
  }

  async function copy(value: string) {
    await navigator.clipboard.writeText(value);
    toast.success("Nusxalandi");
  }

  if (portal && !["admin", "center", "teacher"].includes(portal)) {
    return <Alert>MFA faqat admin, center va teacher accountlari uchun mavjud.</Alert>;
  }

  return (
    <>
      <PageHeader title="Security & MFA" subtitle="Admin, center va teacher accountlari uchun TOTP ikki bosqichli himoya." />
      <div className="grid grid-2 section">
        <Card>
          <div className="row-between">
            <div>
              <h3>Account assurance</h3>
              <div className="muted">Login va muhim yozuvchi amallarda AAL2 himoyasi ishlaydi.</div>
            </div>
            <Pill>{status?.aal || "—"}</Pill>
          </div>
          <div className="section row"><ShieldCheck size={20} /><b>{status?.enabled ? "TOTP yoqilgan" : "TOTP hali yoqilmagan"}</b></div>
          {status?.verified_at ? <div className="muted">Verified: {new Date(status.verified_at).toLocaleString()}</div> : null}
        </Card>
        <Card>
          <h3>Authenticator</h3>
          <p className="muted">Google Authenticator, Microsoft Authenticator, 1Password, Authy yoki boshqa TOTP ilovasidan foydalaning.</p>
          {!status?.enabled && !setup ? <Button onClick={start} disabled={busy}><KeyRound size={16} />MFA setup boshlash</Button> : null}
          {status?.enabled ? <>
            <Field label="6 xonali joriy kod"><Input inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} placeholder="123456" /></Field>
            <div style={{ marginTop: 12 }}><Button variant="outline" onClick={disable} disabled={busy || code.trim().length < 6}><ShieldOff size={16} />MFA o‘chirish</Button></div>
          </> : null}
        </Card>
      </div>

      {setup ? <Card className="section">
        <h3>1. QR kodni skanerlang</h3>
        <p className="muted">QR kod brauzer ichida lokal yaratiladi. TOTP secret hech qanday tashqi QR servisiga yuborilmaydi.</p>
        <div className="grid grid-2 section" style={{ alignItems: "center" }}>
          <div style={{ display: "grid", placeItems: "center" }}><MfaQr value={setup.otpauth_uri} /></div>
          <div>
            <div className="kicker">Account</div>
            <div className="codebox mono">{setup.account}</div>
            <div className="kicker" style={{ marginTop: 12 }}>Manual setup key</div>
            <div className="codebox mono">{setup.secret}</div>
            <Button variant="ghost" onClick={() => copy(setup.secret)}><Copy size={15} />Secretni nusxalash</Button>
          </div>
        </div>
        <details className="section">
          <summary>Advanced: otpauth URI</summary>
          <div className="codebox mono" style={{ marginTop: 10, overflowWrap: "anywhere" }}>{setup.otpauth_uri}</div>
          <Button variant="ghost" onClick={() => copy(setup.otpauth_uri)}><Copy size={15} />URI ni nusxalash</Button>
        </details>
        <div className="divider" />
        <h3>2. Authenticator kodini tasdiqlang</h3>
        <div className="row" style={{ alignItems: "end", flexWrap: "wrap" }}>
          <Field label="6 xonali kod"><Input inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} placeholder="123456" /></Field>
          <Button onClick={verify} disabled={busy || code.trim().length < 6}>Setupni tasdiqlash</Button>
        </div>
      </Card> : null}

      {recovery.length ? <Card className="section">
        <h3>Recovery codes</h3>
        <Alert>Bu kodlar faqat bir marta ko‘rsatiladi. Ularni password manager yoki xavfsiz offline joyga saqlang.</Alert>
        <div className="grid grid-2 section">{recovery.map((item) => <div className="codebox mono" key={item}>{item}</div>)}</div>
        <Button variant="outline" onClick={() => copy(recovery.join("\n"))}><Copy size={15} />Barchasini nusxalash</Button>
      </Card> : null}
    </>
  );
}
