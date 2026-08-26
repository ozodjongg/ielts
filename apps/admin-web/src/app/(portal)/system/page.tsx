"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Button, Card, Field, Input, PageHeader, Pill } from "@/components/ui";

type ReadyState = { status: string; modules?: Record<string, { ok: boolean; error?: string; latency_ms?: number }> };

export default function SystemPage() {
  const auth = useAuth();
  const router = useRouter();
  const [ready, setReady] = useState<ReadyState | null>(null);
  const [readyError, setReadyError] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");

  useEffect(() => {
    let active = true;
    void api<ReadyState>("/ready")
      .then((result) => { if (active) { setReady(result); setReadyError(""); } })
      .catch((error: Error) => { if (active) setReadyError(error.message); });
    return () => { active = false; };
  }, []);

  const modules = ready?.modules || {};
  return (
    <>
      <PageHeader title="System readiness" subtitle="Backend module health and administrator account security." />
      <div className="grid grid-2 section">
        <Card>
          <h3>Backend modules</h3>
          {readyError ? <div className="error">{readyError}</div> : ready ? (
            <><div className="row-between"><b>Overall status</b><Pill tone={ready.status === "ready" ? "ok" : "bad"}>{ready.status}</Pill></div><div className="divider"/><div className="stack">{Object.entries(modules).map(([name, value]) => <div key={name} className="row-between"><span>{name}</span><div className="row"><span className="muted">{value.latency_ms ?? 0} ms</span><Pill tone={value.ok ? "ok" : "bad"}>{value.ok ? "ready" : value.error || "down"}</Pill></div></div>)}</div></>
          ) : <div className="muted">Checking…</div>}
        </Card>
        <Card>
          <h3>Change admin password</h3>
          <div className="stack">
            <Field label="Current password"><Input type="password" autoComplete="current-password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} /></Field>
            <Field label="New password"><Input type="password" autoComplete="new-password" minLength={10} value={newPassword} onChange={(e) => setNewPassword(e.target.value)} /></Field>
            <Button className="primary" disabled={currentPassword.length < 1 || newPassword.length < 10} onClick={async () => {
              try {
                await api(portalPath("admin", "identity", "/me/password"), auth.session!.access_token, json("PATCH", { current_password: currentPassword, new_password: newPassword }));
                toast.success("Password changed. Please sign in again.");
                await auth.logout();
                router.replace("/login");
              } catch (error: any) { toast.error(error.message); }
            }}>Change password</Button>
            <div className="muted" style={{ fontSize: 12 }}>Changing the password revokes every active session for this administrator.</div>
          </div>
        </Card>
      </div>
    </>
  );
}
