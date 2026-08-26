"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Alert, Button, Card, Empty, PageHeader, Pill, Progress } from "@/components/ui";
import { api, apiBlob, json, portalPath } from "@/lib/api";

type ListeningAssignment = {
  id: string;
  set_id: string;
  target_type: string;
  due_at?: string | null;
};

type ListeningQuestion = {
  id: string;
  prompt: string;
  options: string[];
};

type AttemptView = {
  title?: string;
  play_count?: number;
  max_plays?: number;
  allow_seek?: boolean;
  questions?: ListeningQuestion[];
};

type AttemptStart = { attempt_id?: string; id?: string };
type PlayToken = { audio_id: string; token: string };
type FinishResult = { correct: number; total: number; score: number };

export default function ListeningPage() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [assignments, setAssignments] = useState<ListeningAssignment[]>([]);
  const [attemptID, setAttemptID] = useState("");
  const [current, setCurrent] = useState<AttemptView | null>(null);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [audioUrl, setAudioUrl] = useState("");
  const [result, setResult] = useState<FinishResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);

  const loadAssignments = useCallback(async () => {
    if (!token) return;
    const response = await api<{ items: ListeningAssignment[] }>(
      portalPath("student", "listening", "/assignments"),
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
        if (!cancelled) toast.error(error instanceof Error ? error.message : "Listening assignments could not be loaded");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [loadAssignments, token]);

  useEffect(() => {
    return () => {
      if (audioUrl) URL.revokeObjectURL(audioUrl);
    };
  }, [audioUrl]);

  const loadAttempt = useCallback(
    async (id: string) => {
      if (!token || !id) return;
      const response = await api<AttemptView>(
        portalPath("student", "listening", `/attempts/${id}`),
        token,
      );
      setCurrent(response);
    },
    [token],
  );

  const start = async (id: string) => {
    if (!token || !id) {
      toast.error("Listening assignment ID is missing.");
      return;
    }
    try {
      setBusy(true);
      const response = await api<AttemptStart>(
        portalPath("student", "listening", `/assignments/${id}/start`),
        token,
        json("POST", {}),
      );
      const nextAttemptID = response.attempt_id ?? response.id;
      if (!nextAttemptID) throw new Error("Backend did not return a listening attempt ID.");
      setAttemptID(nextAttemptID);
      setAnswers({});
      setResult(null);
      await loadAttempt(nextAttemptID);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Listening assessment could not be started");
    } finally {
      setBusy(false);
    }
  };

  const recordEvent = useCallback(
    async (type: string, positionMs?: number) => {
      if (!attemptID || !token) return;
      try {
        await api(
          portalPath("student", "listening", `/attempts/${attemptID}/events`),
          token,
          json("POST", { event_type: type, position_ms: positionMs }),
        );
      } catch {
        // Playback telemetry must never break the test UI.
      }
    },
    [attemptID, token],
  );

  const play = async () => {
    if (!attemptID || !token) return;
    try {
      setBusy(true);
      const playback = await api<PlayToken>(
        portalPath("student", "listening", `/attempts/${attemptID}/play-token`),
        token,
        json("POST", {}),
      );
      if (!playback.audio_id || !playback.token) throw new Error("Playback authorization is incomplete.");
      const blob = await apiBlob(
        portalPath("student", "listening", `/audio/${playback.audio_id}/stream?attempt_id=${encodeURIComponent(attemptID)}`),
        token,
        { "X-Playback-Token": playback.token },
      );
      setAudioUrl((previous) => {
        if (previous) URL.revokeObjectURL(previous);
        return URL.createObjectURL(blob);
      });
      setCurrent((previous) => previous ? { ...previous, play_count: (previous.play_count ?? 0) + 1 } : previous);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Audio playback could not be authorized");
    } finally {
      setBusy(false);
    }
  };

  const finish = async () => {
    if (!attemptID || !token || !current) return;
    const questions = current.questions ?? [];
    if (Object.keys(answers).length !== questions.length) {
      toast.error("Answer every listening question first.");
      return;
    }
    try {
      setBusy(true);
      const response = await api<FinishResult>(
        portalPath("student", "listening", `/attempts/${attemptID}/finish`),
        token,
        json("POST", { answers }),
      );
      setResult(response);
      setCurrent(null);
      setAttemptID("");
      setAnswers({});
      setAudioUrl((previous) => {
        if (previous) URL.revokeObjectURL(previous);
        return "";
      });
      toast.success("Listening completed");
      await loadAssignments();
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Listening assessment could not be finished");
    } finally {
      setBusy(false);
    }
  };

  if (current && attemptID) {
    const questions = current.questions ?? [];
    const answered = Object.keys(answers).length;
    const maxPlays = current.max_plays ?? 2;
    const playCount = current.play_count ?? 0;
    return (
      <>
        <PageHeader
          title={current.title ?? "Listening test"}
          subtitle={`Plays: ${playCount}/${maxPlays} · Answers: ${answered}/${questions.length}`}
        />
        <Progress className="section" value={questions.length ? (answered / questions.length) * 100 : 0} />
        <Card className="section p-6">
          <div className="row-between">
            <Pill>{current.allow_seek ? "Seek enabled" : "No seeking"}</Pill>
            <Button onClick={() => void play()} disabled={busy || playCount >= maxPlays}>
              Authorize next playback
            </Button>
          </div>
          {audioUrl ? (
            <SecureAudio src={audioUrl} allowSeek={Boolean(current.allow_seek)} onEvent={recordEvent} />
          ) : (
            <Alert className="mt-4">Playback is private and attempt-bound. Authorize playback before listening.</Alert>
          )}
          <div className="divider" />
          {questions.map((question, questionIndex) => (
            <section key={question.id} className="section" aria-labelledby={`listening-question-${question.id}`}>
              <h3 id={`listening-question-${question.id}`} className="text-base font-semibold">
                {questionIndex + 1}. {question.prompt}
              </h3>
              <div className="stack section">
                {question.options.map((option, optionIndex) => {
                  const selected = answers[question.id] === option;
                  return (
                    <Button
                      key={`${question.id}-${optionIndex}`}
                      variant={selected ? "default" : "outline"}
                      className="question-option h-auto min-h-12 justify-start whitespace-normal px-4 py-3 text-left"
                      aria-pressed={selected}
                      onClick={() => setAnswers((value) => ({ ...value, [question.id]: option }))}
                    >
                      <b>{String.fromCharCode(65 + optionIndex)}.</b>
                      <span>{option}</span>
                    </Button>
                  );
                })}
              </div>
            </section>
          ))}
          <Button className="section" disabled={busy || questions.length === 0} onClick={() => void finish()}>
            Finish listening
          </Button>
        </Card>
      </>
    );
  }

  return (
    <>
      <PageHeader title="Listening assignments" subtitle="Audio playback is attempt-bound, tokenized and play-count limited." />
      {result ? (
        <Alert className="section">
          Completed: {result.correct}/{result.total} · score {Number(result.score).toFixed(1)}%
        </Alert>
      ) : null}
      {loading ? <Card className="section p-6"><span className="muted">Listening assignments loading…</span></Card> : null}
      {!loading && assignments.length === 0 ? <Empty>No listening assignments are currently available.</Empty> : null}
      <div className="grid grid-2 section">
        {assignments.map((assignment) => (
          <Card className="p-6" key={assignment.id}>
            <div className="row-between">
              <b>Listening assignment</b>
              <Pill>{assignment.target_type}</Pill>
            </div>
            <p className="muted mt-4">
              Set #{assignment.set_id.slice(0, 10)}
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

function SecureAudio({
  src,
  allowSeek,
  onEvent,
}: {
  src: string;
  allowSeek: boolean;
  onEvent: (type: string, positionMs?: number) => Promise<void>;
}) {
  const lastPosition = useRef(0);
  return (
    <audio
      controls
      controlsList="nodownload noplaybackrate"
      preload="metadata"
      src={src}
      aria-label="Protected listening audio"
      onContextMenu={(event) => event.preventDefault()}
      onPlay={(event) => void onEvent("play", Math.round(event.currentTarget.currentTime * 1000))}
      onPause={(event) => void onEvent("pause", Math.round(event.currentTarget.currentTime * 1000))}
      onEnded={(event) => void onEvent("ended", Math.round(event.currentTarget.currentTime * 1000))}
      onTimeUpdate={(event) => {
        lastPosition.current = event.currentTarget.currentTime;
      }}
      onSeeking={(event) => {
        if (!allowSeek && Math.abs(event.currentTarget.currentTime - lastPosition.current) > 0.8) {
          void onEvent("seek_attempt", Math.round(event.currentTarget.currentTime * 1000));
          event.currentTarget.currentTime = lastPosition.current;
        }
      }}
      className="mt-5 w-full"
    />
  );
}
