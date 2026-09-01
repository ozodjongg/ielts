"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ClipboardCheck } from "lucide-react";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Empty, Input, PageHeader, Pill, Select, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap } from "@/components/ui";

type Student = { user_id: string; full_name: string; email: string; current_level?: string | null; status: string };
const levels = ["A1", "A2", "B1", "B2", "C1", "C2"];

export default function StudentsPage() {
  const auth = useAuth();
  const router = useRouter();
  const [items, setItems] = useState<Student[]>([]);
  const [resetPasswords, setResetPasswords] = useState<Record<string, string>>({});

  const load = useCallback(async () => {
    if (!auth.session) return;
    const result = await api<{ items: Student[] }>(portalPath("center", "tenant", "/students"), auth.session.access_token);
    setItems(result.items || []);
  }, [auth.session]);
  useEffect(() => { void load().catch((error: Error) => toast.error(error.message)); }, [load]);

  async function updateStudent(id: string, payload: Record<string, unknown>, success: string) {
    if (!auth.session) return;
    try { await api(portalPath("center", "tenant", `/students/${id}`), auth.session.access_token, json("PATCH", payload)); toast.success(success); await load(); } catch (error: any) { toast.error(error.message); }
  }

  return <>
    <PageHeader title="Students" subtitle="Student accounts, access status, learning level and password recovery." action={<Button className="accent" onClick={() => router.push("/placement")}><ClipboardCheck size={16}/>Yangi student: placement test</Button>}/>
    <Alert className="section"><b>Yangi student uchun darajani qo‘lda tanlash shart emas.</b> Avval placement test o‘tkaziladi; natija chiqqach akkaunt shu daraja bilan avtomatik yaratiladi. Telefonsiz nomzod uchun printerga tayyor Word rejimi ham mavjud.</Alert>
    <div className="section">{items.length === 0 ? <Empty>Hali studentlar yo‘q. Yangi studentni placement testdan boshlang.</Empty> : <TableWrap><Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Email</TableHead><TableHead>Level</TableHead><TableHead>Status</TableHead><TableHead>New temporary password</TableHead></TableRow></TableHeader><TableBody>{items.map((student) => <TableRow key={student.user_id}><TableCell><b>{student.full_name}</b><div className="mono muted">{student.user_id.slice(0,12)}</div></TableCell><TableCell>{student.email}</TableCell><TableCell><Select value={student.current_level || "A1"} onChange={(e) => void updateStudent(student.user_id, { current_level: e.target.value }, "Level updated")}>{levels.map((value) => <option key={value}>{value}</option>)}</Select></TableCell><TableCell><Select value={student.status} onChange={(e) => void updateStudent(student.user_id, { status: e.target.value }, "Status updated")}><option value="active">active</option><option value="suspended">suspended</option><option value="archived">archived</option></Select><div style={{marginTop:6}}><Pill tone={student.status === "active" ? "ok" : "bad"}>{student.status}</Pill></div></TableCell><TableCell><div className="row"><Input type="password" autoComplete="new-password" minLength={10} placeholder="10+ characters" value={resetPasswords[student.user_id] || ""} onChange={(e) => setResetPasswords((current) => ({...current,[student.user_id]:e.target.value}))}/><Button disabled={(resetPasswords[student.user_id] || "").length < 10} onClick={async () => { const password = resetPasswords[student.user_id] || ""; await updateStudent(student.user_id, { new_password: password }, "Password reset; student sessions revoked"); setResetPasswords((current) => ({...current,[student.user_id]:""})); }}>Reset</Button></div></TableCell></TableRow>)}</TableBody></Table></TableWrap>}</div>
  </>;
}
