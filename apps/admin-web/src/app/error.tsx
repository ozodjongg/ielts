"use client";

import { useEffect } from "react";
import { Button, Card } from "@/components/ui";

export default function GlobalRouteError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error("admin-web route error", error);
  }, [error]);

  return (
    <div className="login-wrap">
      <Card className="login-card">
        <div className="brand"><span className="brandmark">IELTS</span><span>IELTS Admin</span></div>
        <h1 className="title">Sahifa yuklanmadi</h1>
        <p className="subtitle">Admin portalda kutilmagan xato yuz berdi.</p>
        {error.digest ? <p className="mono section">Error ID: {error.digest}</p> : null}
        <Button className="section" onClick={reset}>Qayta urinish</Button>
      </Card>
    </div>
  );
}
