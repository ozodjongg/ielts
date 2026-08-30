"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import {
  Alert,
  Button,
  Card,
  Empty,
  Input,
  PageHeader,
  Pill,
  Progress,
  Textarea,
} from "@/components/ui";
import { api, json, portalPath } from "@/lib/api";

type Assignment = {
  id: string;
  service_code: string;
  title: string;
  status: string;
  question_count?: number | null;
  due_at?: string | null;
};

type ManualPrompt = {
  prompt_id: string;
  component: "speaking" | "writing";
  position: number;
  prompt_text: string;
  required: boolean;
  submitted: boolean;
};

type Question = {
  id: string;
  text: string;
  options: string[];
  rush_multiplier?: number;
};

type AttemptView = {
  attempt_id?: string;
  status: string;
  service_code?: string;
  answered?: number;
  total?: number;
  question?: Question;
  ready_to_finish?: boolean;
  manual_prompts?: ManualPrompt[];
};

type FinishResult = {
  status?: string;
  final_score?: number | null;
  auto_score?: number | null;
  score?: number | null;
  level_result?: string | null;
  level?: string | null;
};

const responseLabel = ["Again", "Hard", "Poor", "OK", "Good", "Easy"];
void responseLabel;

export default function EnglishServicesPanel() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [attemptID, setAttemptID] = useState("");
  const [current, setCurrent] = useState<AttemptView | null>(null);
  const [result, setResult] = useState<FinishResult | null>(null);
  const [writing, setWriting] = useState<Record<string, string>>({});
  const [audio, setAudio] = useState<Record<string, File | null>>({});
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);

  const loadAssignments = useCallback(async () => {
    if (!token) return;
    const response = await api<{ items: Assignment[] }>(
      portalPath("student", "assessment", "/assignments"),
      token,
    );
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
        if (!cancelled) toast.error(error instanceof Error ? error.message : "Assignments could not be loaded");
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
      if (!token || !id) return null;
      const response = await api<AttemptView>(
        portalPath("student", "assessment", `/attempts/${id}`),
        token,
      );
      setCurrent(response);
      return response;
    },
    [token],
  );

  const start = async (assignmentID: string) => {
    if (!token || !assignmentID) {
      toast.error("Assignment ID is missing.");
      return;
    }
    try {
      setBusy(true);
      setResult(null);
      const response = await api<{ attempt_id?: string; id?: string }>(
        portalPath("student", "assessment", `/assignments/${assignmentID}/start`),
        token,
        json("POST", {}),
      );
      const id = response.attempt_id ?? response.id;
      if (!id) throw new Error("Backend did not return an attempt ID.");
      setAttemptID(id);
      await loadCurrent(id);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Assessment could not be started");
    } finally {
      setBusy(false);
    }
  };

  const finish = async () => {
    if (!token || !attemptID) return;
    try {
      setBusy(true);
      const response = await api<FinishResult>(
        portalPath("student", "assessment", `/attempts/${attemptID}/finish`),
        token,
        json("POST", {}),
      );
      setResult(response);
      setCurrent(null);
      setAttemptID("");
      setWriting({});
      setAudio({});
      await loadAssignments();
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Assessment could not be finished");
    } finally {
      setBusy(false);
    }
  };

  const answer = async (option: string) => {
    if (!token || !attemptID || !current?.question) return;
    try {
      setBusy(true);
      await api(
        portalPath("student", "assessment", `/attempts/${attemptID}/answer`),
        token,
        json("POST", {
          question_id: current.question.id,
          option,
          response_ms: 1200,
        }),
      );
      const next = await loadCurrent(attemptID);
      if (next?.ready_to_finish && !(next.manual_prompts ?? []).length) {
        await finish();
      }
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Answer could not be submitted");
    } finally {
      setBusy(false);
    }
  };

  const submitWriting = async (prompt: ManualPrompt) => {
    if (!token || !attemptID || !current?.service_code) return;
    const text = (writing[prompt.prompt_id] ?? "").trim();
    if (text.length < 20) {
      toast.error("Writing response is too short.");
      return;
    }
    try {
      setBusy(true);
      await api(
        portalPath("student", "review", "/submissions/text"),
        token,
        json("POST", {
          attempt_id: attemptID,
          service_code: current.service_code,
          prompt_id: prompt.prompt_id,
          text,
        }),
      );
      toast.success("Writing response submitted");
      await loadCurrent(attemptID);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Writing response could not be submitted");
    } finally {
      setBusy(false);
    }
  };

  const submitSpeaking = async (prompt: ManualPrompt) => {
    if (!token || !attemptID || !current?.service_code) return;
    const file = audio[prompt.prompt_id];
    if (!file) {
      toast.error("Choose or record an audio response first.");
      return;
    }
    try {
      setBusy(true);
      const body = new FormData();
      body.set("attempt_id", attemptID);
      body.set("service_code", current.service_code);
      body.set("prompt_id", prompt.prompt_id);
      body.set("audio", file);
      await api(portalPath("student", "review", "/submissions/audio"), token, {
        method: "POST",
        body,
      });
      toast.success("Speaking response submitted");
      await loadCurrent(attemptID);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Speaking response could not be submitted");
    } finally {
      setBusy(false);
    }
  };

  const manualPrompts = useMemo(() => current?.manual_prompts ?? [], [current?.manual_prompts]);

  if (current?.question) {
    const answered = current.answered ?? 0;
    const total = Math.max(1, current.total ?? 80);
    return (
      <>
        <PageHeader title="English assessment" subtitle={`${answered} / ${total} answered`} />
        <Progress className="section" value={(answered / total) * 100} />
        <Card className="section p-6">
          <div className="row-between">
            <Pill>{current.service_code ?? "English"}</Pill>
            <Pill>Rush ×{Number(current.question.rush_multiplier ?? 1).toFixed(2)}</Pill>
          </div>
          <h2 className="mt-6 text-xl font-semibold leading-relaxed">{current.question.text}</h2>
          <div className="stack section">
            {current.question.options.map((option, index) => (
              <Button
                key={`${index}-${option}`}
                variant="outline"
                className="question-option h-auto min-h-12 justify-start whitespace-normal px-4 py-3 text-left"
                disabled={busy}
                onClick={() => void answer(String.fromCharCode(65 + index))}
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

  if (current && manualPrompts.length > 0) {
    const allRequiredSubmitted = manualPrompts.every((prompt) => !prompt.required || prompt.submitted);
    return (
      <>
        <PageHeader
          title={current.service_code === "mock" ? "Mock — manual components" : `${current.service_code ?? "English"} assessment`}
          subtitle="Submit every required response. Correct answers and scoring rubrics remain server-side."
        />
        <div className="stack section">
          {manualPrompts.map((prompt) => (
            <Card className="p-6" key={prompt.prompt_id}>
              <div className="row-between">
                <div>
                  <Pill>{prompt.component}</Pill>
                  <h3 className="mt-3 text-lg font-semibold">
                    {prompt.component === "speaking" ? "Speaking prompt" : "Writing prompt"} {prompt.position}
                  </h3>
                </div>
                <Pill>{prompt.submitted ? "Submitted" : prompt.required ? "Required" : "Optional"}</Pill>
              </div>
              <p className="mt-4 text-base leading-relaxed">{prompt.prompt_text}</p>
              {!prompt.submitted && prompt.component === "writing" ? (
                <div className="stack section">
                  <Textarea
                    value={writing[prompt.prompt_id] ?? ""}
                    onChange={(event) => setWriting((value) => ({ ...value, [prompt.prompt_id]: event.target.value }))}
                    placeholder="Write your response here…"
                    aria-label={`Writing response ${prompt.position}`}
                  />
                  <Button disabled={busy} onClick={() => void submitWriting(prompt)}>
                    Submit writing
                  </Button>
                </div>
              ) : null}
              {!prompt.submitted && prompt.component === "speaking" ? (
                <div className="stack section">
                  <Input
                    type="file"
                    accept="audio/webm,audio/ogg,audio/mpeg,audio/mp4,audio/wav"
                    aria-label={`Speaking audio ${prompt.position}`}
                    onChange={(event) => setAudio((value) => ({ ...value, [prompt.prompt_id]: event.target.files?.[0] ?? null }))}
                  />
                  <p className="muted">Audio is sent through the authenticated API to private storage. The server enforces the maximum size.</p>
                  <Button disabled={busy || !audio[prompt.prompt_id]} onClick={() => void submitSpeaking(prompt)}>
                    Submit speaking audio
                  </Button>
                </div>
              ) : null}
            </Card>
          ))}
        </div>
        <Card className="section p-6">
          <div className="row-between">
            <div>
              <b>{allRequiredSubmitted ? "Ready to finish" : "Responses still required"}</b>
              <div className="muted mt-1">After finishing, the center reviewer receives the manual components.</div>
            </div>
            <Button disabled={!allRequiredSubmitted || busy} onClick={() => void finish()}>
              Finish assessment
            </Button>
          </div>
        </Card>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="English assessments"
        subtitle="Placement, vocabulary, progress, diagnostics, level upgrades, speaking, writing and hybrid mock tests."
      />
      {result ? (
        <Alert className="section">
          {result.status === "pending_review"
            ? "Submitted for center review. Your final score will appear after manual review."
            : `Completed. Score: ${result.final_score ?? result.auto_score ?? result.score ?? "—"} · Level: ${result.level_result ?? result.level ?? "—"}`}
        </Alert>
      ) : null}
      {loading ? <Card className="section p-6"><span className="muted">Assignments loading…</span></Card> : null}
      {!loading && assignments.length === 0 ? <Empty>No English assignments are currently available.</Empty> : null}
      <div className="grid grid-2 section">
        {assignments.map((assignment) => (
          <Card className="p-6" key={assignment.id}>
            <div className="row-between">
              <div>
                <b>{assignment.title}</b>
                <div className="muted mt-1">{assignment.service_code}</div>
              </div>
              <Pill>{assignment.status}</Pill>
            </div>
            <p className="muted mt-4">
              {assignment.question_count ?? "Adaptive"} questions/prompts
              {assignment.due_at ? ` · Due ${new Date(assignment.due_at).toLocaleDateString()}` : ""}
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
