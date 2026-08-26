"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, Field, Input, PageHeader, Pill, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap } from "@/components/ui";

type Student = { user_id: string; full_name: string; email: string; current_level?: string | null; status: string };
const levels = ["A1", "A2", "B1", "B2", "C1", "C2"];

export default function StudentsPage() {
  const auth = useAuth();
  const [items, setItems] = useState<Student[]>([]);
  const [form, setForm] = useState({ email: "", password: "", full_name: "", current_level: "A1" });
  const [resetPasswords, setResetPasswords] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    if (!auth.session) return;
    const result = await api<{ items: Student[] }>(portalPath("center", "tenant", "/students"), auth.session.access_token);
    setItems(result.items || []);
  }, [auth.session]);
  useEffect(() => { void load().catch((error: Error) => toast.error(error.message)); }, [load]);

  async function createStudent() {
    if (!auth.session || busy) return;
    setBusy(true);
    try {
      await api(portalPath("center", "tenant", "/students"), auth.session.access_token, json("POST", form));
      toast.success("Student created"); setForm({ ...form, email: "", password: "", full_name: "" }); await load();
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  async function updateStudent(id: string, payload: Record<string, unknown>, success: string) {
    if (!auth.session) return;
    try { await api(portalPath("center", "tenant", `/students/${id}`), auth.session.access_token, json("PATCH", payload)); toast.success(success); await load(); } catch (error: any) { toast.error(error.message); }
  }

  return <>
    <PageHeader title="Students" subtitle="Student accounts, access status, learning level and password recovery."/>
    <Card className="section"><div className="grid grid-4"><Field label="Full name"><Input required value={form.full_name} onChange={(e) => setForm({...form,full_name:e.target.value})}/></Field><Field label="Email"><Input required type="email" value={form.email} onChange={(e) => setForm({...form,email:e.target.value})}/></Field><Field label="Temporary password"><Input required type="password" autoComplete="new-password" minLength={10} value={form.password} onChange={(e) => setForm({...form,password:e.target.value})}/></Field><Field label="Initial level"><Select value={form.current_level} onChange={(e) => setForm({...form,current_level:e.target.value})}>{levels.map((value) => <option key={value}>{value}</option>)}</Select></Field></div><Button className="accent section" onClick={() => void createStudent()} disabled={busy || !form.full_name.trim() || !form.email.trim() || form.password.length < 10}>{busy ? "Creating…" : "Add student"}</Button></Card>
    <div className="section">{items.length === 0 ? <Empty>No students yet.</Empty> : <TableWrap><Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Email</TableHead><TableHead>Level</TableHead><TableHead>Status</TableHead><TableHead>New temporary password</TableHead></TableRow></TableHeader><TableBody>{items.map((student) => <TableRow key={student.user_id}><TableCell><b>{student.full_name}</b><div className="mono muted">{student.user_id.slice(0,12)}</div></TableCell><TableCell>{student.email}</TableCell><TableCell><Select value={student.current_level || "A1"} onChange={(e) => void updateStudent(student.user_id, { current_level: e.target.value }, "Level updated")}>{levels.map((value) => <option key={value}>{value}</option>)}</Select></TableCell><TableCell><Select value={student.status} onChange={(e) => void updateStudent(student.user_id, { status: e.target.value }, "Status updated")}><option value="active">active</option><option value="suspended">suspended</option><option value="archived">archived</option></Select><div style={{marginTop:6}}><Pill tone={student.status === "active" ? "ok" : "bad"}>{student.status}</Pill></div></TableCell><TableCell><div className="row"><Input type="password" autoComplete="new-password" minLength={10} placeholder="10+ characters" value={resetPasswords[student.user_id] || ""} onChange={(e) => setResetPasswords((current) => ({...current,[student.user_id]:e.target.value}))}/><Button disabled={(resetPasswords[student.user_id] || "").length < 10} onClick={async () => { const password = resetPasswords[student.user_id] || ""; await updateStudent(student.user_id, { new_password: password }, "Password reset; student sessions revoked"); setResetPasswords((current) => ({...current,[student.user_id]:""})); }}>Reset</Button></div></TableCell></TableRow>)}</TableBody></Table></TableWrap>}</div>
  </>;
}
