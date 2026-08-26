"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Field, Input } from "@/components/ui";

export default function LoginPage() {
  const auth = useAuth();
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (auth.profile) router.replace("/");
  }, [auth.profile, router]);

  return (
    <div className="login-wrap">
      <div className="login-card">
        <div className="brand"><span className="brandmark">V5</span><span>V5 Learning Center</span></div>
        <h1 className="title">Kirish</h1>
        <p className="subtitle">Learning-center administration portal</p>
        <form
          className="stack section"
          onSubmit={async (event) => {
            event.preventDefault();
            if (busy) return;
            setBusy(true);
            setError("");
            try {
              await auth.login(email.trim(), password);
              router.replace("/");
            } catch (cause: unknown) {
              setError(cause instanceof Error ? cause.message : "Login xatosi");
            } finally {
              setBusy(false);
            }
          }}
        >
          <Field label="Email">
            <Input type="email" inputMode="email" autoComplete="email" required maxLength={254} value={email} onChange={(event) => setEmail(event.target.value)} />
          </Field>
          <Field label="Parol">
            <Input type="password" autoComplete="current-password" required minLength={10} maxLength={128} value={password} onChange={(event) => setPassword(event.target.value)} />
          </Field>
          {error || auth.error ? <Alert>{error || auth.error}</Alert> : null}
          <Button type="submit" disabled={busy || auth.loading}>{busy ? "Tekshirilmoqda…" : "Portalga kirish"}</Button>
        </form>
        <div className="divider" />
        <p className="muted" style={{ fontSize: 12 }}>Authentication V5 backend tomonidan boshqariladi; sessiya, rol va ruxsatlar server-side tekshiriladi.</p>
      </div>
    </div>
  );
}
