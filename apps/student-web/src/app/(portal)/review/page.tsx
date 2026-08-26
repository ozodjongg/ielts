"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Clock3, Search, Sparkles, Target } from "lucide-react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Button, Card, Empty, PageHeader, Pill, Progress } from "@/components/ui";
import { api, json, portalPath } from "@/lib/api";

type Lexeme = {
  index: number;
  english: string;
  uzbek: unknown;
  part_of_speech?: string | null;
  cefr: string;
};

type ReviewItem = {
  word: Lexeme;
  search_count: number;
  review_count: number;
  correct_count: number;
  incorrect_count: number;
  interval_minutes: number;
  mastery: number;
  next_review_at: string;
  last_review_at?: string | null;
  status: "learning" | "mastered" | "suspended";
};

type DueResponse = {
  items: ReviewItem[];
  due_count: number;
  next_review_at?: string | null;
};

type Stats = {
  learned: number;
  average_mastery: number;
  due_now: number;
  searched_words: number;
  total_searches: number;
  reviews: number;
  mastered: number;
  next_review_at?: string | null;
};

type GradeResponse = {
  interval_minutes: number;
  next_review_at: string;
  mastery: number;
  status: string;
};

type Rating = "again" | "hard" | "good" | "easy";

function translations(value: unknown) {
  if (Array.isArray(value)) return value.map(String).join(", ");
  if (typeof value === "string") return value;
  if (value && typeof value === "object") return Object.values(value as Record<string, unknown>).map(String).join(", ");
  return "—";
}

function intervalLabel(minutes: number) {
  if (minutes < 60) return `${minutes} min`;
  if (minutes < 1440) return `${Math.round(minutes / 60)} soat`;
  const days = Math.round(minutes / 1440);
  return `${days} kun`;
}

function nextLabel(value?: string | null) {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat("uz-UZ", { dateStyle: "medium", timeStyle: "short" }).format(d);
}

const ratings: Array<{ value: Rating; label: string; hint: string }> = [
  { value: "again", label: "Again", hint: "Esdan chiqdi" },
  { value: "hard", label: "Hard", hint: "Qiyin bo‘ldi" },
  { value: "good", label: "Good", hint: "Esladim" },
  { value: "easy", label: "Easy", hint: "Juda oson" },
];

export default function ReviewPage() {
  const auth = useAuth();
  const token = auth.session?.access_token;
  const [items, setItems] = useState<ReviewItem[]>([]);
  const [dueCount, setDueCount] = useState(0);
  const [initialQueueCount, setInitialQueueCount] = useState(0);
  const [nextReviewAt, setNextReviewAt] = useState<string | null>(null);
  const [stats, setStats] = useState<Stats | null>(null);
  const [revealed, setRevealed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const [due, summary] = await Promise.all([
        api<DueResponse>(portalPath("student", "vocabulary", "/review/due?limit=50"), token),
        api<Stats>(portalPath("student", "vocabulary", "/stats"), token),
      ]);
      setItems(due.items ?? []);
      setDueCount(due.due_count ?? 0);
      setInitialQueueCount((due.items ?? []).length);
      setNextReviewAt(due.next_review_at ?? summary.next_review_at ?? null);
      setStats(summary);
      setRevealed(false);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Review queue yuklanmadi");
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => { void load(); }, [load]);

  const current = items[0];
  const completedThisQueue = useMemo(() => Math.max(0, initialQueueCount - items.length), [initialQueueCount, items.length]);
  const progress = initialQueueCount > 0 ? (completedThisQueue / initialQueueCount) * 100 : 0;

  const rate = async (rating: Rating) => {
    if (!token || !current) return;
    setBusy(true);
    try {
      const result = await api<GradeResponse>(
        portalPath("student", "vocabulary", `/review/${current.word.index}/grade`),
        token,
        json("POST", { rating }),
      );
      toast.success(`${current.word.english}: keyingi review ${intervalLabel(result.interval_minutes)} dan keyin`);
      setItems((prev) => prev.slice(1));
      setRevealed(false);
      const summary = await api<Stats>(portalPath("student", "vocabulary", "/stats"), token);
      setStats(summary);
      setNextReviewAt(summary.next_review_at ?? null);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Review natijasi saqlanmadi");
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <PageHeader
        title="Vocabulary review"
        subtitle="Lug‘atda qidirgan so‘zlaringiz 90 daqiqadan keyin boshlanadigan intervalli takrorlashga avtomatik qo‘shiladi."
        action={<Button variant="outline" disabled={loading || busy} onClick={() => void load()}>Yangilash</Button>}
      />

      <div className="grid grid-4 section">
        <Card className="p-5"><div className="muted row"><Clock3 size={15} />Hozir due</div><div className="metric">{stats?.due_now ?? dueCount}</div></Card>
        <Card className="p-5"><div className="muted row"><Search size={15} />Qidirilgan so‘zlar</div><div className="metric">{stats?.searched_words ?? 0}</div></Card>
        <Card className="p-5"><div className="muted row"><Target size={15} />Reviewlar</div><div className="metric">{stats?.reviews ?? 0}</div></Card>
        <Card className="p-5"><div className="muted row"><Sparkles size={15} />Mastered</div><div className="metric">{stats?.mastered ?? 0}</div></Card>
      </div>

      {dueCount > 0 ? <Progress className="section" value={progress} /> : null}

      {loading ? (
        <div className="section"><Empty>Review queue yuklanmoqda…</Empty></div>
      ) : current ? (
        <Card className="section review-card p-7">
          <div className="row-between">
            <div className="row flex-wrap">
              <Pill>{current.word.cefr}</Pill>
              <Pill>{current.word.part_of_speech ?? "word"}</Pill>
              {current.status === "mastered" ? <Pill>Mastered</Pill> : null}
            </div>
            <span className="muted text-sm">Qidirilgan: {current.search_count} · Review: {current.review_count}</span>
          </div>

          <div className="review-prompt">
            <div className="kicker">English → Uzbek</div>
            <h2>{current.word.english}</h2>
            {!revealed ? (
              <>
                <p className="muted">Uzbek ma’nosini eslang, keyin javobni oching.</p>
                <Button size="lg" onClick={() => setRevealed(true)}>Javobni ko‘rsatish</Button>
              </>
            ) : (
              <>
                <div className="review-answer">{translations(current.word.uzbek)}</div>
                <p className="muted">O‘zingizni baholang. Javob sifati keyingi intervalni belgilaydi.</p>
              </>
            )}
          </div>

          {revealed ? (
            <div className="review-ratings">
              {ratings.map((rating) => (
                <Button
                  key={rating.value}
                  variant={rating.value === "good" || rating.value === "easy" ? "default" : "outline"}
                  disabled={busy}
                  onClick={() => void rate(rating.value)}
                >
                  <span>{rating.label}</span><span className="rating-hint">{rating.hint}</span>
                </Button>
              ))}
            </div>
          ) : null}
        </Card>
      ) : (
        <Card className="section p-6">
          <Empty>
            Hozir review uchun so‘z yo‘q. Dictionary’da so‘z qidiring; yangi so‘z birinchi marta taxminan 90 daqiqadan keyin so‘raladi.
            {nextReviewAt ? <div className="mt-3">Keyingi review: <b>{nextLabel(nextReviewAt)}</b></div> : null}
          </Empty>
        </Card>
      )}
    </>
  );
}
