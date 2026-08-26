"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, Input, PageHeader, Pill, Progress } from "@/components/ui";
import { api, json, portalPath } from "@/lib/api";

type Lexeme = {
  index: number;
  english: string;
  uzbek: unknown;
  part_of_speech?: string | null;
  cefr: string;
  level_source?: string;
  frequency_rank?: number | null;
  synonym_group_id?: number | null;
};

type DailyItem = {
  position: number;
  word: Lexeme;
  is_review: boolean;
  mastery: number;
};

type DailySession = {
  session_id: string;
  level: string;
  new_count: number;
  review_count: number;
  completed_at?: string | null;
  items: DailyItem[];
};

type Synonym = { word: Lexeme; weight: number };
type ReviewState = {
  lexeme_index: number;
  search_count: number;
  next_review_at: string;
};

function translations(value: unknown) {
  if (Array.isArray(value)) return value.map(String).join(", ");
  if (typeof value === "string") return value;
  if (value && typeof value === "object") return Object.values(value as Record<string, unknown>).map(String).join(", ");
  return "—";
}

const gradeLabels = ["Again", "Hard", "Poor", "OK", "Good", "Easy"];

export default function VocabularyPage() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [daily, setDaily] = useState<DailySession | null>(null);
  const [position, setPosition] = useState(0);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Lexeme[]>([]);
  const [synonyms, setSynonyms] = useState<Synonym[]>([]);
  const [lastEnrolledIndex, setLastEnrolledIndex] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [dailyError, setDailyError] = useState("");

  const loadDaily = useCallback(async () => {
    if (!token) return;
    try {
      setDailyError("");
      const response = await api<DailySession>(portalPath("student", "vocabulary", "/daily"), token);
      setDaily(response);
      setPosition((current) => Math.min(current, Math.max(0, (response.items?.length ?? 1) - 1)));
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : "Daily vocabulary could not be loaded";
      setDailyError(message);
      setDaily(null);
      throw error;
    }
  }, [token]);

  useEffect(() => {
    if (!token) return;
    let active = true;
    void loadDaily().catch((error: unknown) => {
      if (active) toast.error(error instanceof Error ? error.message : "Daily vocabulary could not be loaded");
    });
    return () => {
      active = false;
    };
  }, [loadDaily, token]);

  const current = daily?.items?.[position];
  const completion = useMemo(() => {
    if (!daily?.items?.length) return 0;
    return ((position + (current ? 0 : 1)) / daily.items.length) * 100;
  }, [current, daily?.items?.length, position]);

  const grade = async (value: number) => {
    if (!token || !daily || !current) return;
    try {
      setBusy(true);
      await api(
        portalPath("student", "vocabulary", `/daily/${daily.session_id}/grade`),
        token,
        json("POST", { position: current.position, grade: value }),
      );
      if (position + 1 < daily.items.length) {
        setPosition((next) => next + 1);
      } else {
        toast.success("Daily vocabulary completed");
        setPosition(0);
        await loadDaily();
      }
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Vocabulary grade could not be saved");
    } finally {
      setBusy(false);
    }
  };

  const search = async () => {
    if (!token || query.trim().length < 1) return;
    try {
      setBusy(true);
      const response = await api<{ items: Lexeme[]; review_enrolled?: ReviewState | null }>(
        portalPath("student", "vocabulary", `/search?q=${encodeURIComponent(query.trim())}&limit=30`),
        token,
      );
      setResults(response.items ?? []);
      setSynonyms([]);
      setLastEnrolledIndex(response.review_enrolled?.lexeme_index ?? null);
      if (response.review_enrolled?.search_count === 1) {
        toast.success("So‘z reviewga qo‘shildi. Birinchi takrorlash ~90 daqiqadan keyin.");
      }
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Dictionary search failed");
    } finally {
      setBusy(false);
    }
  };

  const openSynonyms = async (index: number) => {
    if (!token || !Number.isFinite(index)) return;
    try {
      const response = await api<{ items: Synonym[] }>(portalPath("student", "vocabulary", `/words/${index}/synonyms`), token);
      let state: ReviewState | null = null;
      if (lastEnrolledIndex !== index) {
        state = await api<ReviewState>(portalPath("student", "vocabulary", `/words/${index}/seen`), token, json("POST", {}));
        setLastEnrolledIndex(index);
      }
      setSynonyms(response.items ?? []);
      if (state?.search_count === 1) toast.success("So‘z reviewga qo‘shildi.");
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Synonyms could not be loaded");
    }
  };

  return (
    <>
      <PageHeader title="Daily vocabulary" subtitle="Level-matched new words plus spaced-repetition reviews." />
      {daily?.items?.length ? <Progress className="section" value={completion} /> : null}
      {current ? (
        <Card className="section max-w-3xl p-6">
          <div className="row-between">
            <Pill>{current.word.cefr}</Pill>
            <span className="muted">{position + 1}/{daily?.items.length ?? 0} · {current.is_review ? "Review" : "New"}</span>
          </div>
          <h2 className="mt-6 text-4xl font-semibold tracking-tight">{current.word.english}</h2>
          <div className="mt-2 text-lg">{translations(current.word.uzbek)}</div>
          <p className="muted mt-2">{current.word.part_of_speech ?? "word"} · index #{current.word.index}</p>
          <div className="divider" />
          <div className="row flex-wrap">
            {gradeLabels.map((label, value) => (
              <Button key={label} variant={value >= 3 ? "default" : "outline"} disabled={busy} onClick={() => void grade(value)}>
                {label}
              </Button>
            ))}
          </div>
        </Card>
      ) : (
        <div className="section"><Empty>{dailyError || "Daily session is loading or already completed."}</Empty></div>
      )}

      <div className="section"><PageHeader title="Dictionary" subtitle="Search EN→UZ. Exact searches and opened words are automatically added to spaced review." /></div>
      <Card className="section p-6">
        <div className="row">
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => { if (event.key === "Enter") void search(); }}
            placeholder="Search an English word"
            aria-label="Dictionary search"
          />
          <Button disabled={busy || !query.trim()} onClick={() => void search()}>Search</Button>
        </div>
      </Card>
      {results.length === 0 ? null : (
        <div className="grid grid-2 section">
          {results.map((item) => (
            <Card className="cursor-pointer p-6" key={item.index} onClick={() => void openSynonyms(item.index)} role="button" tabIndex={0} onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") void openSynonyms(item.index); }}>
              <div className="row-between"><b>{item.english}</b><Pill>{item.cefr}</Pill></div>
              <p className="mt-3">{translations(item.uzbek)}</p>
              <div className="muted mt-3">#{item.index} · open synonyms</div>
            </Card>
          ))}
        </div>
      )}
      {synonyms.length > 0 ? (
        <Card className="section p-6">
          <h3 className="text-lg font-semibold">Synonyms</h3>
          <div className="row mt-4 flex-wrap">
            {synonyms.map((item) => <Pill key={item.word.index}>{item.word.english}</Pill>)}
          </div>
        </Card>
      ) : null}
    </>
  );
}
