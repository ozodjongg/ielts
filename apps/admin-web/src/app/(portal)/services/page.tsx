"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, Field, Input, PageHeader, Select, Switch } from "@/components/ui";

type Center = { id: string; name: string };
type Limit = { service_code: string; name: string; unit: string; enabled: boolean; monthly_limit: number | string; daily_limit: number | string | null; concurrency_limit: number | string };

export default function ServiceLimitsPage() {
  const auth = useAuth();
  const [centers, setCenters] = useState<Center[]>([]);
  const [center, setCenter] = useState("");
  const [limits, setLimits] = useState<Limit[]>([]);

  useEffect(() => {
    if (!auth.session) return;
    let active = true;
    void api<{ items: Center[] }>(portalPath("admin", "tenant", "/centers"), auth.session.access_token)
      .then((result) => {
        if (!active) return;
        setCenters(result.items || []);
        setCenter((current) => current || result.items?.[0]?.id || "");
      })
      .catch((error: Error) => { if (active) toast.error(error.message); });
    return () => { active = false; };
  }, [auth.session]);

  const loadLimits = useCallback(async () => {
    if (!auth.session || !center) { setLimits([]); return; }
    const result = await api<{ items: Limit[] }>(portalPath("admin", "tenant", `/centers/${center}/services`), auth.session.access_token);
    setLimits(result.items || []);
  }, [auth.session, center]);

  useEffect(() => {
    void loadLimits().catch((error: Error) => toast.error(error.message));
  }, [loadLimits]);

  function patch(index: number, values: Partial<Limit>) {
    setLimits((current) => current.map((item, i) => i === index ? { ...item, ...values } : item));
  }

  async function update(item: Limit) {
    if (!auth.session || !center) return;
    try {
      await api(portalPath("admin", "tenant", `/centers/${center}/services/${item.service_code}`), auth.session.access_token, json("PATCH", {
        enabled: item.enabled,
        monthly_limit: Number(item.monthly_limit),
        daily_limit: item.daily_limit === "" || item.daily_limit == null ? undefined : Number(item.daily_limit),
        clear_daily_limit: item.daily_limit === "" || item.daily_limit == null,
        concurrency_limit: Number(item.concurrency_limit),
      }));
      toast.success(`${item.name} limit saved`);
      await loadLimits();
    } catch (error: any) { toast.error(error.message); }
  }

  return (
    <>
      <PageHeader title="Service limits" subtitle="Monthly, daily and concurrency quotas per learning center." />
      <div className="section" style={{ maxWidth: 520 }}><Field label="Center"><Select value={center} onChange={(e) => setCenter(e.target.value)}><option value="">Select center…</option>{centers.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</Select></Field></div>
      {center && limits.length === 0 ? <Empty>No service limits found.</Empty> : (
        <div className="section grid grid-2">
          {limits.map((item, index) => (
            <Card key={item.service_code}>
              <div className="row-between"><div><b>{item.name}</b><div className="muted mono">{item.service_code} · {item.unit}</div></div><Switch checked={item.enabled} onCheckedChange={(checked) => patch(index, { enabled: checked })} aria-label={`Enable ${item.name}`} /></div>
              <div className="grid grid-3 section">
                <Field label="Monthly"><Input type="number" min={0} value={item.monthly_limit} onChange={(e) => patch(index, { monthly_limit: e.target.value })} /></Field>
                <Field label="Daily"><Input type="number" min={0} value={item.daily_limit ?? ""} onChange={(e) => patch(index, { daily_limit: e.target.value })} /></Field>
                <Field label="Concurrency"><Input type="number" min={1} value={item.concurrency_limit} onChange={(e) => patch(index, { concurrency_limit: e.target.value })} /></Field>
              </div>
              <Button className="primary section" onClick={() => void update(item)}>Save</Button>
            </Card>
          ))}
        </div>
      )}
    </>
  );
}
