"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, Field, Input, PageHeader, Pill, Select } from "@/components/ui";

type Group = { id: string; name: string; level?: string | null; member_count: number };
type Student = { user_id: string; full_name: string; email: string };
type Teacher = { user_id: string; full_name: string; email: string; status: string };
type Member = { student_user_id: string; user_id?: string; full_name?: string; email?: string };
type GroupTeacher = { teacher_user_id: string; user_id?: string; full_name?: string; email?: string; status?: string };

export default function GroupsPage() {
  const auth = useAuth();
  const [groups, setGroups] = useState<Group[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [name, setName] = useState("");
  const [level, setLevel] = useState("A1");
  const [teacherIDs, setTeacherIDs] = useState<string[]>([]);

  const load = useCallback(async () => {
    if (!auth.session) return;
    const token = auth.session.access_token;
    const [g, s, t] = await Promise.all([
      api<{ items: Group[] }>(portalPath("center", "tenant", "/groups"), token),
      api<{ items: Student[] }>(portalPath("center", "tenant", "/students"), token),
      api<{ items: Teacher[] }>(portalPath("center", "tenant", "/teachers"), token),
    ]);
    setGroups(g.items || []); setStudents(s.items || []); setTeachers((t.items || []).filter((x) => x.status === "active"));
  }, [auth.session]);

  useEffect(() => { void load().catch((error: Error) => toast.error(error.message)); }, [load]);

  async function createGroup() {
    if (!auth.session || !name.trim()) return;
    try {
      await api(portalPath("center", "tenant", "/groups"), auth.session.access_token, json("POST", { name: name.trim(), level, teacher_user_ids: teacherIDs }));
      setName(""); setTeacherIDs([]); toast.success("Group created"); await load();
    } catch (error: any) { toast.error(error.message); }
  }

  return <>
    <PageHeader title="Groups" subtitle="Studentlar va bir yoki bir nechta teacherni groupga bog‘lang. Teacher faqat o‘z grouplarini boshqaradi." />
    <Card className="section">
      <div className="grid grid-3">
        <Field label="Group name"><Input value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Level"><Select value={level} onChange={(e) => setLevel(e.target.value)}>{["A1", "A2", "B1", "B2", "C1", "C2"].map((value) => <option key={value}>{value}</option>)}</Select></Field>
        <div><div className="kicker">Teachers</div><div className="stack" style={{ maxHeight: 150, overflow: "auto", paddingTop: 8 }}>{teachers.length ? teachers.map((teacher) => <label className="row" key={teacher.user_id}><input type="checkbox" checked={teacherIDs.includes(teacher.user_id)} onChange={(e) => setTeacherIDs((current) => e.target.checked ? [...current, teacher.user_id] : current.filter((id) => id !== teacher.user_id))} /><span>{teacher.full_name}</span></label>) : <span className="muted">Avval teacher yarating.</span>}</div></div>
      </div>
      <Button className="accent section" onClick={() => void createGroup()} disabled={!name.trim()}>Create group</Button>
    </Card>
    <div className="grid grid-2 section">{groups.length === 0 ? <Empty>No groups yet.</Empty> : groups.map((group) => <GroupCard key={group.id} group={group} students={students} teachers={teachers} token={auth.session!.access_token} reload={load} />)}</div>
  </>;
}

function GroupCard({ group, students, teachers, token, reload }: { group: Group; students: Student[]; teachers: Teacher[]; token: string; reload: () => Promise<void> }) {
  const [studentID, setStudentID] = useState("");
  const [teacherID, setTeacherID] = useState("");
  const [members, setMembers] = useState<Member[]>([]);
  const [assignedTeachers, setAssignedTeachers] = useState<GroupTeacher[]>([]);

  const loadLinks = useCallback(async () => {
    const [memberResult, teacherResult] = await Promise.all([
      api<{ items: Member[] }>(portalPath("center", "tenant", `/groups/${group.id}/students`), token),
      api<{ items: GroupTeacher[] }>(portalPath("center", "tenant", `/groups/${group.id}/teachers`), token),
    ]);
    setMembers(memberResult.items || []); setAssignedTeachers(teacherResult.items || []);
  }, [group.id, token]);

  useEffect(() => { void loadLinks().catch((error: Error) => toast.error(error.message)); }, [loadLinks]);
  const availableTeachers = teachers.filter((teacher) => !assignedTeachers.some((item) => item.teacher_user_id === teacher.user_id));

  return <Card>
    <div className="row-between"><div><b>{group.name}</b><div className="muted">{group.level || "—"}</div></div><Pill>{members.length} students</Pill></div>

    <div className="divider" /><h4>Teachers</h4>
    <div className="row section"><Select value={teacherID} onChange={(e) => setTeacherID(e.target.value)}><option value="">Select teacher…</option>{availableTeachers.map((teacher) => <option key={teacher.user_id} value={teacher.user_id}>{teacher.full_name}</option>)}</Select><Button disabled={!teacherID} onClick={async () => { try { await api(portalPath("center", "tenant", `/groups/${group.id}/teachers`), token, json("POST", { teacher_user_id: teacherID })); setTeacherID(""); toast.success("Teacher assigned"); await loadLinks(); } catch (error: any) { toast.error(error.message); } }}>Assign</Button></div>
    <div className="stack">{assignedTeachers.length === 0 ? <div className="muted">No teachers assigned.</div> : assignedTeachers.map((teacher) => <div className="row-between" key={teacher.teacher_user_id}><span>{teacher.full_name || teacher.email || teacher.teacher_user_id}</span><Button variant="destructive" onClick={async () => { try { await api(portalPath("center", "tenant", `/groups/${group.id}/teachers/${teacher.teacher_user_id}`), token, { method: "DELETE" }); await loadLinks(); } catch (error: any) { toast.error(error.message); } }}>Remove</Button></div>)}</div>

    <div className="divider" /><h4>Students</h4>
    <div className="row section"><Select value={studentID} onChange={(e) => setStudentID(e.target.value)}><option value="">Select student…</option>{students.map((student) => <option key={student.user_id} value={student.user_id}>{student.full_name}</option>)}</Select><Button disabled={!studentID} onClick={async () => { try { await api(portalPath("center", "tenant", `/groups/${group.id}/students`), token, json("POST", { student_user_id: studentID })); setStudentID(""); toast.success("Student added"); await loadLinks(); await reload(); } catch (error: any) { toast.error(error.message); } }}>Add</Button></div>
    <div className="stack">{members.length === 0 ? <div className="muted">No members.</div> : members.map((member) => <div className="row-between" key={member.student_user_id}><span>{member.full_name || member.email || member.student_user_id}</span><Button variant="destructive" onClick={async () => { try { await api(portalPath("center", "tenant", `/groups/${group.id}/students/${member.student_user_id}`), token, { method: "DELETE" }); await loadLinks(); await reload(); } catch (error: any) { toast.error(error.message); } }}>Remove</Button></div>)}</div>
  </Card>;
}
