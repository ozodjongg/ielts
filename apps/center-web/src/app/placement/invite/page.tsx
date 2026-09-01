"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ShieldCheck } from "lucide-react";
import { api, json } from "@/lib/api";
import { Alert, Card, Loading } from "@/components/ui";

const SESSION_KEY = "ielts-placement-candidate-session";

type ClaimResponse = {
  id: string;
  full_name: string;
  session_token: string;
  session_expires_at: string;
};

type ExistingSession = { token: string; placement_id: string; expires_at: string };

async function sessionStillWorks(saved: ExistingSession) {
  if (!saved?.token || Date.parse(saved.expires_at) <= Date.now()) return false;
  try {
    await api("/public/placement/session", "", { headers: { "X-Placement-Session": saved.token } });
    return true;
  } catch {
    return false;
  }
}

export default function PlacementInvitationPage() {
  const router = useRouter();
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const params = new URLSearchParams(window.location.hash.replace(/^#/, ""));
        const token = (params.get("token") || "").trim();
        // A freshly scanned QR always takes priority over a previously saved session.
        // Remove the bearer token from the visible URL immediately: fragments are not
        // sent to the web server and this also avoids accidental sharing/screenshots.
        window.history.replaceState(null, "", "/placement/invite");

        if (!token) {
          const existingRaw = localStorage.getItem(SESSION_KEY);
          if (existingRaw) {
            try {
              const existing = JSON.parse(existingRaw) as ExistingSession;
              if (await sessionStillWorks(existing)) {
                router.replace("/placement/test");
                return;
              }
            } catch {}
            localStorage.removeItem(SESSION_KEY);
          }
          throw new Error("Invitation token topilmadi. Center bergan QR-kodni qayta skaner qiling.");
        }

        const claimed = await api<ClaimResponse>("/public/placement/invitations/claim", "", json("POST", { token }));
        localStorage.setItem(SESSION_KEY, JSON.stringify({
          token: claimed.session_token,
          placement_id: claimed.id,
          expires_at: claimed.session_expires_at,
        } satisfies ExistingSession));
        if (active) router.replace("/placement/test");
      } catch (e: any) {
        if (active) setError(e?.message || "Invitationni ochib bo‘lmadi.");
      }
    })();
    return () => { active = false; };
  }, [router]);

  return <main className="login-wrap">
    <Card className="login-card" style={{ textAlign: "center" }}>
      <div style={{ display: "grid", placeItems: "center", gap: 12 }}>
        <ShieldCheck size={42} aria-hidden="true" />
        <h1 style={{ margin: 0, fontSize: 24 }}>IELTS Placement Test</h1>
        {!error ? <><Loading label="Xavfsiz invitation tekshirilmoqda…"/><p className="muted">Test shu qurilmaga biriktiriladi.</p></> : <Alert>{error}</Alert>}
      </div>
    </Card>
  </main>;
}
