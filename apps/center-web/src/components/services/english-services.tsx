"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

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
import { api, json, portalPath } from "@/lib/api";

const services = [
  "placement",
  "level_upgrade",
  "progress",
  "grammar",
  "ielts_readiness",
  "weakness",
  "speaking",
  "writing",
  "mock",
  "final_exit",
] as const;

const defaults: Record<string, number> = {
  placement: 80,
  level_upgrade: 40,
  progress: 30,
  grammar: 40,
  ielts_readiness: 40,
  weakness: 30,
  speaking: 3,
  writing: 2,
  mock: 60,
  final_exit: 60,
};

const upgradeNext: Record<string, string> = {
  A1: "A2",
  A2: "B1",
  B1: "B2",
  B2: "C1",
};

type Assignment = {
  id: string;
  title: string;
  service_code: string;
  target_type: string;
  question_count?: number | null;
  status: string;
  due_at?: string | null;
};
type Group = { id: string; name: string };
type Student = { user_id: string; full_name: string };

type FormState = {
  service_code: string;
  title: string;
  target_type: string;
  target_id: string;
  from_level: string;
  to_level: string;
  question_count: number;
  due_at: string;
};

export default function EnglishServicesPanel() {
  const auth = useAuth();
  const [items, setItems] = useState<Assignment[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState<FormState>({
    service_code: "placement",
    title: "Placement test",
    target_type: "all",
    target_id: "",
    from_level: "A1",
    to_level: "A2",
    question_count: 80,
    due_at: "",
  });

  const load = useCallback(async () => {
    if (!auth.session) return;
    const token = auth.session.access_token;
    const [assignments, groupList, studentList] = await Promise.all([
      api<{ items: Assignment[] }>(portalPath("center", "assessment", "/assignments"), token),
      api<{ items: Group[] }>(portalPath("center", "tenant", "/groups"), token),
      api<{ items: Student[] }>(portalPath("center", "tenant", "/students"), token),
    ]);
    setItems(assignments.items ?? []);
    setGroups(groupList.items ?? []);
    setStudents(studentList.items ?? []);
  }, [auth.session]);

  useEffect(() => {
    void load().catch((error: Error) => toast.error(error.message));
  }, [load]);

  const targets = useMemo(
    () => (form.target_type === "group" ? groups : students),
    [form.target_type, groups, students],
  );

  async function createAssignment() {
    if (!auth.session || busy) return;
    if (!form.title.trim()) {
      toast.error("Assignment title is required");
      return;
    }
    if (form.target_type !== "all" && !form.target_id) {
      toast.error("Target item is required");
      return;
    }
    if (!Number.isInteger(form.question_count) || form.question_count < 1 || form.question_count > 80) {
      toast.error("Question count must be between 1 and 80");
      return;
    }
    if (form.service_code === "speaking" && form.question_count > 5) {
      toast.error("Speaking supports up to 5 prompts");
      return;
    }
    if (form.service_code === "writing" && form.question_count > 3) {
      toast.error("Writing supports up to 3 prompts");
      return;
    }

    const body: Record<string, unknown> = {
      service_code: form.service_code,
      title: form.title.trim(),
      target_type: form.target_type,
      question_count: form.question_count,
    };
    if (form.target_type !== "all") body.target_id = form.target_id;
    if (form.service_code === "level_upgrade") {
      body.from_level = form.from_level;
      body.to_level = upgradeNext[form.from_level];
    }
    if (form.due_at) body.due_at = new Date(`${form.due_at}T23:59:59`).toISOString();

    setBusy(true);
    try {
      await api(
        portalPath("center", "assessment", "/assignments"),
        auth.session.access_token,
        json("POST", body),
      );
      toast.success("Assignment created");
      await load();
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Assignment could not be created");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader
        title="English assessments"
        subtitle="Placement, progress, level-up, diagnostics, mock, speaking and writing assessments."
      />
      <Card className="section">
        <div className="grid grid-3">
          <Field label="Service">
            <Select
              value={form.service_code}
              onChange={(event) => {
                const service_code = event.target.value;
                setForm((current) => ({
                  ...current,
                  service_code,
                  question_count: defaults[service_code] ?? 40,
                }));
              }}
            >
              {services.map((value) => <option key={value}>{value}</option>)}
            </Select>
          </Field>
          <Field label="Title">
            <Input value={form.title} maxLength={180} onChange={(event) => setForm({ ...form, title: event.target.value })} />
          </Field>
          <Field label="Target">
            <Select
              value={form.target_type}
              onChange={(event) => setForm({ ...form, target_type: event.target.value, target_id: "" })}
            >
              <option value="all">All students</option>
              <option value="group">Group</option>
              <option value="student">Student</option>
            </Select>
          </Field>
          {form.target_type !== "all" ? (
            <Field label="Target item">
              <Select value={form.target_id} onChange={(event) => setForm({ ...form, target_id: event.target.value })}>
                <option value="">Select…</option>
                {targets.map((item) => {
                  const id = "id" in item ? item.id : item.user_id;
                  const label = "name" in item ? item.name : item.full_name;
                  return <option key={id} value={id}>{label}</option>;
                })}
              </Select>
            </Field>
          ) : null}
          <Field label="Question count">
            <Input
              type="number"
              min={1}
              max={form.service_code === "speaking" ? 5 : form.service_code === "writing" ? 3 : 80}
              value={form.question_count}
              onChange={(event) => setForm({ ...form, question_count: Number(event.target.value) })}
            />
          </Field>
          <Field label="Due date" hint="Optional. The assignment remains open without a due date.">
            <Input
              type="date"
              min={new Date().toISOString().slice(0, 10)}
              value={form.due_at}
              onChange={(event) => setForm({ ...form, due_at: event.target.value })}
            />
          </Field>
          {form.service_code === "level_upgrade" ? (
            <>
              <Field label="From level">
                <Select
                  value={form.from_level}
                  onChange={(event) => {
                    const from_level = event.target.value;
                    setForm({ ...form, from_level, to_level: upgradeNext[from_level] });
                  }}
                >
                  {Object.keys(upgradeNext).map((value) => <option key={value}>{value}</option>)}
                </Select>
              </Field>
              <Field label="To level">
                <Input value={upgradeNext[form.from_level]} readOnly aria-readonly="true" />
              </Field>
            </>
          ) : null}
        </div>
        <Button className="section" disabled={busy} onClick={() => void createAssignment()}>
          {busy ? "Creating…" : "Create assignment"}
        </Button>
      </Card>
      <div className="section">
        {items.length === 0 ? (
          <Empty>No English assignments yet.</Empty>
        ) : (
          <TableWrap>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Title</TableHead>
                  <TableHead>Service</TableHead>
                  <TableHead>Target</TableHead>
                  <TableHead>Questions</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Due</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell><b>{item.title}</b></TableCell>
                    <TableCell>{item.service_code}</TableCell>
                    <TableCell>{item.target_type}</TableCell>
                    <TableCell>{item.question_count ?? "default"}</TableCell>
                    <TableCell><Pill tone={item.status === "open" ? "ok" : ""}>{item.status}</Pill></TableCell>
                    <TableCell>{item.due_at ? new Date(item.due_at).toLocaleDateString() : "—"}</TableCell>
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
