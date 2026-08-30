"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import {
  Alert,
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
  Textarea,
} from "@/components/ui";

type Lexeme = {
  index: number;
  english: string;
  uzbek: string[];
  cefr: string;
  part_of_speech?: string | null;
  source_name: string;
  source_license: string;
};

type CheckResult = {
  english: string;
  exists: boolean;
  added: boolean;
  word?: Lexeme;
  error?: string;
};

type Contribution = {
  created_at: string;
  word: Lexeme;
};

type WordInput = {
  english: string;
  uzbek: string[];
  cefr: string;
  part_of_speech: string;
};

const levels = ["A1", "A2", "B1", "B2", "C1", "C2"];
const posOptions = [
  "noun",
  "verb",
  "adjective",
  "adverb",
  "pronoun",
  "determiner",
  "preposition",
  "conjunction",
  "interjection",
  "numeral",
  "modal",
  "phrase",
  "phrasal_verb",
];

function splitLines(value: string) {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseBulk(value: string): { items: WordInput[]; errors: string[] } {
  const items: WordInput[] = [];
  const errors: string[] = [];

  splitLines(value).forEach((line, index) => {
    const parts = line.split("|").map((item) => item.trim());
    if (parts.length < 3 || parts.length > 4) {
      errors.push(`Line ${index + 1}: english | uzbek | CEFR | POS format required`);
      return;
    }
    const [english, uzbekRaw, cefr, pos = ""] = parts;
    const uzbek = uzbekRaw.split(",").map((item) => item.trim()).filter(Boolean);
    if (!english || uzbek.length === 0 || !levels.includes(cefr.toUpperCase())) {
      errors.push(`Line ${index + 1}: English, Uzbek and valid CEFR are required`);
      return;
    }
    items.push({
      english,
      uzbek,
      cefr: cefr.toUpperCase(),
      part_of_speech: pos.toLowerCase(),
    });
  });

  return { items, errors };
}

export default function VocabularyManagerPage() {
  const auth = useAuth();
  const [english, setEnglish] = useState("");
  const [uzbek, setUzbek] = useState("");
  const [cefr, setCefr] = useState("B1");
  const [pos, setPos] = useState("noun");
  const [checkText, setCheckText] = useState("");
  const [checkResults, setCheckResults] = useState<CheckResult[]>([]);
  const [bulkText, setBulkText] = useState("");
  const [contributions, setContributions] = useState<Contribution[]>([]);
  const [busy, setBusy] = useState(false);
  const [loadingContributions, setLoadingContributions] = useState(false);

  const loadContributions = useCallback(async () => {
    if (!auth.session) return;
    setLoadingContributions(true);
    try {
      const response = await api<{ items: Contribution[] }>(
        portalPath("teacher", "vocabulary", "/teacher/contributions?limit=100"),
        auth.session.access_token,
      );
      setContributions(response.items || []);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Contributions could not be loaded");
    } finally {
      setLoadingContributions(false);
    }
  }, [auth.session]);

  useEffect(() => {
    void loadContributions();
  }, [loadContributions]);

  const missingCount = useMemo(
    () => checkResults.filter((item) => !item.exists && !item.error).length,
    [checkResults],
  );

  async function checkWords() {
    if (!auth.session || busy) return;
    const words = splitLines(checkText);
    if (words.length === 0) {
      toast.error("Enter at least one English word");
      return;
    }
    if (words.length > 200) {
      toast.error("Maximum 200 words can be checked at once");
      return;
    }
    setBusy(true);
    try {
      const response = await api<{ items: CheckResult[] }>(
        portalPath("teacher", "vocabulary", "/teacher/words/check"),
        auth.session.access_token,
        json("POST", { words }),
      );
      setCheckResults(response.items || []);
      const missing = (response.items || []).filter((item) => !item.exists && !item.error).length;
      toast.success(`${missing} missing word${missing === 1 ? "" : "s"} found`);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Vocabulary check failed");
    } finally {
      setBusy(false);
    }
  }

  async function addSingle() {
    if (!auth.session || busy) return;
    const translations = uzbek.split(",").map((item) => item.trim()).filter(Boolean);
    if (!english.trim() || translations.length === 0) {
      toast.error("English and Uzbek translation are required");
      return;
    }
    setBusy(true);
    try {
      const response = await api<CheckResult>(
        portalPath("teacher", "vocabulary", "/teacher/words"),
        auth.session.access_token,
        json("POST", {
          english: english.trim(),
          uzbek: translations,
          cefr,
          part_of_speech: pos,
        }),
      );
      if (response.exists) {
        toast.info("This word already exists in the vocabulary table");
      } else {
        toast.success("Word added to the vocabulary table");
        setEnglish("");
        setUzbek("");
      }
      await loadContributions();
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Word could not be added");
    } finally {
      setBusy(false);
    }
  }

  async function addBulk() {
    if (!auth.session || busy) return;
    const parsed = parseBulk(bulkText);
    if (parsed.errors.length > 0) {
      toast.error(parsed.errors[0]);
      return;
    }
    if (parsed.items.length === 0) {
      toast.error("Add at least one valid row");
      return;
    }
    if (parsed.items.length > 100) {
      toast.error("Maximum 100 rows can be added in one batch");
      return;
    }
    setBusy(true);
    try {
      const response = await api<{ added: number; existing: number; failed: number; items: CheckResult[] }>(
        portalPath("teacher", "vocabulary", "/teacher/words/batch"),
        auth.session.access_token,
        json("POST", { items: parsed.items }),
      );
      toast.success(`Added ${response.added}; already existed ${response.existing}; failed ${response.failed}`);
      if (response.failed === 0) setBulkText("");
      await loadContributions();
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Bulk vocabulary import failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader
        title="Vocabulary Manager"
        subtitle="Check missing English words and add verified EN→UZ vocabulary for your learning center. Existing global words are never duplicated."
      />

      <Alert className="section">
        Center-added vocabulary becomes available to the shared learning corpus. Add lexical words or short phrases only; do not add full sentences. Verify translation accuracy and content rights before submission.
      </Alert>

      <div className="grid grid-2 section">
        <Card>
          <h3>Add one word</h3>
          <div className="stack" style={{ marginTop: 14 }}>
            <Field label="English word / short phrase">
              <Input value={english} onChange={(event) => setEnglish(event.target.value)} placeholder="e.g. achieve" />
            </Field>
            <Field label="Uzbek translation(s)" hint="Separate multiple translations with commas.">
              <Input value={uzbek} onChange={(event) => setUzbek(event.target.value)} placeholder="erishmoq, qo‘lga kiritmoq" />
            </Field>
            <div className="grid grid-2">
              <Field label="CEFR">
                <Select value={cefr} onChange={(event) => setCefr(event.target.value)}>
                  {levels.map((level) => <option key={level} value={level}>{level}</option>)}
                </Select>
              </Field>
              <Field label="Part of speech">
                <Select value={pos} onChange={(event) => setPos(event.target.value)}>
                  {posOptions.map((value) => <option key={value} value={value}>{value}</option>)}
                </Select>
              </Field>
            </div>
            <Button className="accent" disabled={busy || !english.trim() || !uzbek.trim()} onClick={() => void addSingle()}>
              {busy ? "Saving…" : "Add if missing"}
            </Button>
          </div>
        </Card>

        <Card>
          <h3>Check missing words</h3>
          <div className="stack" style={{ marginTop: 14 }}>
            <Field label="English words" hint="One word or short phrase per line; maximum 200.">
              <Textarea value={checkText} onChange={(event) => setCheckText(event.target.value)} placeholder={"improve\nachieve\naccurate\nlook after"} />
            </Field>
            <Button disabled={busy || !checkText.trim()} onClick={() => void checkWords()}>
              {busy ? "Checking…" : "Check vocabulary table"}
            </Button>
          </div>
        </Card>
      </div>

      {checkResults.length > 0 && (
        <div className="section">
          <div className="row" style={{ justifyContent: "space-between", marginBottom: 10 }}>
            <h3>Check results</h3>
            <Pill>{missingCount} missing</Pill>
          </div>
          <TableWrap>
            <Table>
              <TableHeader>
                <TableRow><TableHead>English</TableHead><TableHead>Status</TableHead><TableHead>Current entry</TableHead></TableRow>
              </TableHeader>
              <TableBody>
                {checkResults.map((item, index) => (
                  <TableRow key={`${item.english}-${index}`}>
                    <TableCell><b>{item.english}</b></TableCell>
                    <TableCell>
                      <Pill tone={item.exists ? "ok" : ""}>{item.error ? "Invalid" : item.exists ? "Exists" : "Missing"}</Pill>
                    </TableCell>
                    <TableCell>
                      {item.error
                        ? item.error
                        : item.word
                          ? `${Array.isArray(item.word.uzbek) ? item.word.uzbek.join(", ") : item.word.uzbek} · ${item.word.cefr}`
                          : "Add a verified translation below or with bulk import."}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableWrap>
        </div>
      )}

      <Card className="section">
        <h3>Bulk add missing vocabulary</h3>
        <div className="stack" style={{ marginTop: 14 }}>
          <Field label="Rows" hint="Format: English | Uzbek translation(s) | CEFR | POS. Multiple Uzbek meanings may be comma-separated. Maximum 100 rows.">
            <Textarea
              value={bulkText}
              onChange={(event) => setBulkText(event.target.value)}
              placeholder={"achieve | erishmoq, qo‘lga kiritmoq | B1 | verb\naccurate | aniq | B2 | adjective\nlook after | g‘amxo‘rlik qilmoq | A2 | phrasal_verb"}
            />
          </Field>
          <Button className="accent" disabled={busy || !bulkText.trim()} onClick={() => void addBulk()}>
            {busy ? "Importing…" : "Add only missing words"}
          </Button>
        </div>
      </Card>

      <div className="section">
        <div className="row" style={{ justifyContent: "space-between", marginBottom: 10 }}>
          <h3>Your teacher contributions</h3>
          <Button variant="outline" disabled={loadingContributions} onClick={() => void loadContributions()}>Refresh</Button>
        </div>
        {loadingContributions && contributions.length === 0 ? (
          <Empty>Loading contributions…</Empty>
        ) : contributions.length === 0 ? (
          <Empty>No teacher vocabulary contributions yet.</Empty>
        ) : (
          <TableWrap>
            <Table>
              <TableHeader>
                <TableRow><TableHead>English</TableHead><TableHead>Uzbek</TableHead><TableHead>Level</TableHead><TableHead>POS</TableHead><TableHead>Added</TableHead></TableRow>
              </TableHeader>
              <TableBody>
                {contributions.map((item) => (
                  <TableRow key={`${item.word.index}-${item.created_at}`}>
                    <TableCell><b>{item.word.english}</b></TableCell>
                    <TableCell>{Array.isArray(item.word.uzbek) ? item.word.uzbek.join(", ") : String(item.word.uzbek)}</TableCell>
                    <TableCell><Pill>{item.word.cefr}</Pill></TableCell>
                    <TableCell>{item.word.part_of_speech || "—"}</TableCell>
                    <TableCell>{new Date(item.created_at).toLocaleDateString()}</TableCell>
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
