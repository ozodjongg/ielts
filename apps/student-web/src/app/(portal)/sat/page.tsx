"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Card, Empty, PageHeader, Pill, Progress } from "@/components/ui";
import { api, json, portalPath } from "@/lib/api";

type Assignment = {
  id: string;
  title: string;
  question_count: number;
  due_at?: string | null;
};

type CurrentQuestion = {
  status: string;
  answered: number;
  total: number;
  question_ref?: string;
  topic_code?: string;
  domain?: string;
  prompt?: string;
  options?: string[];
  difficulty?: number;
  rush_multiplier?: number;
  complete?: boolean;
};

type FinishResult = {
  status: string;
  raw_correct: number;
  total: number;
  percent: number;
  estimated_sat_score: number;
};

export default function SATPage() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [attemptID, setAttemptID] = useState("");
  const [current, setCurrent] = useState<CurrentQuestion | null>(null);
  const [result, setResult] = useState<FinishResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);

  const loadAssignments = useCallback(async () => {
    if (!token) return;
    const response = await api<{ items: Assignment[] }>(portalPath("student", "sat", "/assignments"), token);
    setAssignments(response.items ?? []);
  }, [token]);

  useEffect(() => {
    if (!token) {
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    void loadAssignments()
      .catch((error: unknown) => {
        if (!cancelled) toast.error(error instanceof Error ? error.message : "SAT assignments could not be loaded");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [loadAssignments, token]);

  const loadCurrent = useCallback(
    async (id: string) => {
      if (!token || !id) return;
      const response = await api<CurrentQuestion>(portalPath("student", "sat", `/attempts/${id}`), token);
      setCurrent(response);
    },
    [token],
  );

  const finish = async (id: string) => {
    if (!token || !id) return;
    const response = await api<FinishResult>(
      portalPath("student", "sat", `/attempts/${id}/finish`),
      token,
      json("POST", {}),
    );
    setResult(response);
    setCurrent(null);
    setAttemptID("");
    await loadAssignments();
  };

  const start = async (id: string) => {
    if (!token || !id) {
      toast.error("SAT assignment ID is missing.");
      return;
    }
    try {
      setBusy(true);
      setResult(null);
      const response = await api<{ attempt_id?: string; id?: string }>(
        portalPath("student", "sat", `/assignments/${id}/start`),
        token,
        json("POST", {}),
      );
      const nextAttemptID = response.attempt_id ?? response.id;
      if (!nextAttemptID) throw new Error("Backend did not return a SAT attempt ID.");
      setAttemptID(nextAttemptID);
      await loadCurrent(nextAttemptID);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "SAT assessment could not be started");
    } finally {
      setBusy(false);
    }
  };

  const answer = async (displayedOption: number) => {
    if (!token || !attemptID || !current?.question_ref) return;
    try {
      setBusy(true);
      const response = await api<{ remaining: number }>(
        portalPath("student", "sat", `/attempts/${attemptID}/answer`),
        token,
        json("POST", {
          question_ref: current.question_ref,
          displayed_option: displayedOption,
          response_ms: 1500,
        }),
      );
      if (response.remaining === 0) {
        await finish(attemptID);
      } else {
        await loadCurrent(attemptID);
      }
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "SAT answer could not be submitted");
    } finally {
      setBusy(false);
    }
  };

  if (current?.question_ref && current.prompt) {
    const total = Math.max(1, current.total ?? 1);
    const answered = current.answered ?? 0;
    return (
      <>
        <PageHeader title="SAT Math practice" subtitle={`${answered}/${total} answered`} />
        <Progress className="section" value={(answered / total) * 100} />
        <Card className="section p-6">
          <div className="row-between">
            <Pill>{current.domain ?? current.topic_code ?? "SAT Math"}</Pill>
            <Pill>Difficulty {current.difficulty ?? "—"}/10</Pill>
          </div>
          <h2 className="mt-6 text-xl font-semibold leading-relaxed">{current.prompt}</h2>
          <div className="stack section">
            {(current.options ?? []).map((option, index) => (
              <Button
                key={`${index}-${option}`}
                variant="outline"
                className="question-option h-auto min-h-12 justify-start whitespace-normal px-4 py-3 text-left"
                disabled={busy}
                onClick={() => void answer(index)}
              >
                <b>{String.fromCharCode(65 + index)}.</b>
                <span>{option}</span>
              </Button>
            ))}
          </div>
        </Card>
      </>
    );
  }

  return (
    <>
      <PageHeader title="SAT Math" subtitle="Original English SAT-style practice bank." />
      {result ? (
        <Alert className="section">
          Completed: {result.raw_correct}/{result.total} · {Number(result.percent).toFixed(1)}% · estimated practice score {result.estimated_sat_score}
        </Alert>
      ) : null}
      {loading ? <Card className="section p-6"><span className="muted">SAT assignments loading…</span></Card> : null}
      {!loading && assignments.length === 0 ? <Empty>No SAT Math assignments are currently available.</Empty> : null}
      <div className="grid grid-2 section">
        {assignments.map((assignment) => (
          <Card className="p-6" key={assignment.id}>
            <b>{assignment.title}</b>
            <p className="muted mt-3">
              {assignment.question_count} questions
              {assignment.due_at ? ` · due ${new Date(assignment.due_at).toLocaleDateString()}` : ""}
            </p>
            <Button className="mt-4" disabled={busy || !assignment.id} onClick={() => void start(assignment.id)}>
              Start / Continue
            </Button>
          </Card>
        ))}
      </div>
    </>
  );
}
