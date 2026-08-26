"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, Field, Input, PageHeader, Pill, Select } from "@/components/ui";

type Group = { id: string; name: string; level?: string | null; teacher_name?: string | null; member_count: number };
type Student = { user_id: string; full_name: string; email: string };
type Member = { student_user_id: string; user_id?: string; full_name?: string; email?: string };

export default function GroupsPage() {
  const auth = useAuth();
  const [groups, setGroups] = useState<Group[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [name, setName] = useState("");
  const [level, setLevel] = useState("A1");
  const [teacher, setTeacher] = useState("");

  const load = useCallback(async () => {
    if (!auth.session) return;
    const [g, s] = await Promise.all([
      api<{ items: Group[] }>(portalPath("center", "tenant", "/groups"), auth.session.access_token),
      api<{ items: Student[] }>(portalPath("center", "tenant", "/students"), auth.session.access_token),
    ]);
    setGroups(g.items || []); setStudents(s.items || []);
  }, [auth.session]);

  useEffect(() => { void load().catch((error: Error) => toast.error(error.message)); }, [load]);

  async function createGroup() {
    if (!auth.session || !name.trim()) return;
    try {
      await api(portalPath("center", "tenant", "/groups"), auth.session.access_token, json("POST", { name: name.trim(), level, teacher_name: teacher.trim() || null }));
      setName(""); toast.success("Group created"); await load();
    } catch (error: any) { toast.error(error.message); }
  }

  return <>
    <PageHeader title="Groups" subtitle="Tenant-scoped groups that can be targeted by assignments." />
    <Card className="section"><div className="grid grid-3"><Field label="Group name"><Input value={name} onChange={(e) => setName(e.target.value)} /></Field><Field label="Level"><Select value={level} onChange={(e) => setLevel(e.target.value)}>{["A1", "A2", "B1", "B2", "C1", "C2"].map((value) => <option key={value}>{value}</option>)}</Select></Field><Field label="Teacher"><Input value={teacher} onChange={(e) => setTeacher(e.target.value)} /></Field></div><Button className="accent section" onClick={() => void createGroup()} disabled={!name.trim()}>Create group</Button></Card>
    <div className="grid grid-2 section">{groups.length === 0 ? <Empty>No groups yet.</Empty> : groups.map((group) => <GroupCard key={group.id} group={group} students={students} token={auth.session!.access_token} reload={load} />)}</div>
  </>;
}

function GroupCard({ group, students, token, reload }: { group: Group; students: Student[]; token: string; reload: () => Promise<void> }) {
  const [studentID, setStudentID] = useState("");
  const [members, setMembers] = useState<Member[]>([]);

  const loadMembers = useCallback(async () => {
    const result = await api<{ items: Member[] }>(portalPath("center", "tenant", `/groups/${group.id}/students`), token);
    setMembers(result.items || []);
  }, [group.id, token]);

  useEffect(() => { setStudentID((current) => current || students[0]?.user_id || ""); }, [students]);
  useEffect(() => { void loadMembers().catch((error: Error) => toast.error(error.message)); }, [loadMembers]);

  return <Card>
    <div className="row-between"><div><b>{group.name}</b><div className="muted">{group.level || "—"} · {group.teacher_name || "No teacher"}</div></div><Pill>{members.length} students</Pill></div>
    <div className="row section"><Select value={studentID} onChange={(e) => setStudentID(e.target.value)}><option value="">Select student…</option>{students.map((student) => <option key={student.user_id} value={student.user_id}>{student.full_name}</option>)}</Select><Button disabled={!studentID} onClick={async () => { try { await api(portalPath("center", "tenant", `/groups/${group.id}/students`), token, json("POST", { student_user_id: studentID })); toast.success("Student added"); await loadMembers(); await reload(); } catch (error: any) { toast.error(error.message); } }}>Add</Button></div>
    <div className="stack section">{members.length === 0 ? <div className="muted">No members.</div> : members.map((member) => <div className="row-between" key={member.student_user_id}><span>{member.full_name || member.email || member.student_user_id}</span><Button variant="destructive" onClick={async () => { try { await api(portalPath("center", "tenant", `/groups/${group.id}/students/${member.student_user_id}`), token, { method: "DELETE" }); await loadMembers(); await reload(); } catch (error: any) { toast.error(error.message); } }}>Remove</Button></div>)}</div>
  </Card>;
}
