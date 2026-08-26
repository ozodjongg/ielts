"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import {
  Button,
  Card,
  Empty,
  Field,
  Input,
  PageHeader,
  Pill,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableWrap,
} from "@/components/ui";

type Center = {
  id: string;
  name: string;
  slug: string;
  status: string;
  subscription_status: string;
  active_student_limit: number;
  timezone: string;
};

const initialForm = { name: "", slug: "", admin_name: "", admin_email: "", admin_password: "", active_student_limit: "100" };

export default function CentersPage() {
  const auth = useAuth();
  const [items, setItems] = useState<Center[]>([]);
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState(initialForm);

  const load = useCallback(async () => {
    if (!auth.session) return;
    const result = await api<{ items: Center[] }>(portalPath("admin", "tenant", "/centers"), auth.session.access_token);
    setItems(result.items || []);
  }, [auth.session]);

  useEffect(() => {
    void load().catch((error: Error) => toast.error(error.message));
  }, [load]);

  async function createCenter() {
    if (!auth.session || busy) return;
    setBusy(true);
    try {
      await api(portalPath("admin", "tenant", "/centers"), auth.session.access_token, json("POST", {
        ...form,
        active_student_limit: Number(form.active_student_limit),
      }));
      toast.success("Center created");
      setForm(initialForm);
      await load();
    } catch (error: any) {
      toast.error(error.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader title="Learning centers" subtitle="Provision tenants, administrators and service capacity." />
      <div className="grid grid-2 section">
        <Card>
          <h3>New center</h3>
          <div className="grid grid-2">
            <Field label="Center name"><Input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></Field>
            <Field label="Slug"><Input required value={form.slug} onChange={(e) => setForm({ ...form, slug: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "") })} /></Field>
            <Field label="Admin name"><Input required value={form.admin_name} onChange={(e) => setForm({ ...form, admin_name: e.target.value })} /></Field>
            <Field label="Admin email"><Input required type="email" value={form.admin_email} onChange={(e) => setForm({ ...form, admin_email: e.target.value })} /></Field>
            <Field label="Temporary password"><Input required type="password" minLength={10} autoComplete="new-password" value={form.admin_password} onChange={(e) => setForm({ ...form, admin_password: e.target.value })} /></Field>
            <Field label="Active student limit"><Input type="number" min={1} value={form.active_student_limit} onChange={(e) => setForm({ ...form, active_student_limit: e.target.value })} /></Field>
          </div>
          <Button className="accent section" onClick={createCenter} disabled={busy || !form.name || !form.slug || !form.admin_email || form.admin_password.length < 10}>{busy ? "Creating…" : "Create center"}</Button>
        </Card>
        <Card>
          <h3>Provisioning rule</h3>
          <p className="muted">A center becomes active only after its administrator account is successfully created. Default service limits are copied on provisioning and can be overridden per center.</p>
        </Card>
      </div>
      <div className="section">
        {items.length === 0 ? <Empty>No learning centers yet.</Empty> : (
          <TableWrap>
            <Table>
              <TableHeader><TableRow><TableHead>Center</TableHead><TableHead>Status</TableHead><TableHead>Subscription</TableHead><TableHead>Student limit</TableHead><TableHead>Timezone</TableHead><TableHead>Update</TableHead></TableRow></TableHeader>
              <TableBody>
                {items.map((center) => (
                  <TableRow key={center.id}>
                    <TableCell><b>{center.name}</b><div className="muted mono">{center.slug}</div></TableCell>
                    <TableCell><Pill tone={center.status === "active" ? "ok" : "bad"}>{center.status}</Pill></TableCell>
                    <TableCell>{center.subscription_status}</TableCell>
                    <TableCell>{center.active_student_limit}</TableCell>
                    <TableCell>{center.timezone}</TableCell>
                    <TableCell>
                      <Select value={center.status} aria-label={`Update ${center.name} status`} onChange={async (e) => {
                        try {
                          await api(portalPath("admin", "tenant", `/centers/${center.id}`), auth.session!.access_token, json("PATCH", { status: e.target.value }));
                          toast.success("Center updated");
                          await load();
                        } catch (error: any) { toast.error(error.message); }
                      }}>
                        <option value="active">active</option><option value="suspended">suspended</option><option value="archived">archived</option>
                      </Select>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableWrap>
        )}
      </div>
    </>
  );
}
