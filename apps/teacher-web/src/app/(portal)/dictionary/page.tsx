"use client";

import { useState } from "react";
import { toast } from "sonner";
import { api, portalPath } from "@/lib/api";
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

type Lexeme = { index: number; english: string; uzbek: string[]; cefr: string; part_of_speech?: string; source_name: string; source_license: string };
type Synonym = { word: Lexeme; weight: number };

export default function DictionaryPage() {
  const auth = useAuth();
  const [query, setQuery] = useState("");
  const [level, setLevel] = useState("");
  const [items, setItems] = useState<Lexeme[]>([]);
  const [synonyms, setSynonyms] = useState<Synonym[]>([]);
  const [busy, setBusy] = useState(false);

  async function search() {
    if (!auth.session || busy) return;
    setBusy(true);
    try {
      const result = await api<{ items: Lexeme[] }>(portalPath("teacher", "vocabulary", `/search?q=${encodeURIComponent(query.trim())}&level=${encodeURIComponent(level)}&limit=50`), auth.session.access_token);
      setItems(result.items || []);
      setSynonyms([]);
    } catch (error: any) { toast.error(error.message); } finally { setBusy(false); }
  }

  async function openSynonyms(index: number) {
    if (!auth.session) return;
    try {
      const result = await api<{ items: Synonym[] }>(portalPath("teacher", "vocabulary", `/words/${index}/synonyms`), auth.session.access_token);
      setSynonyms(result.items || []);
    } catch (error: any) { toast.error(error.message); }
  }

  return (
    <>
      <PageHeader title="Teacher dictionary" subtitle="Search the indexed EN→UZ learning corpus with CEFR, provenance and synonym graph." />
      <Card className="section">
        <div className="grid grid-3">
          <Field label="English word"><Input value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") void search(); }} placeholder="e.g. improve" /></Field>
          <Field label="CEFR"><Select value={level} onChange={(e) => setLevel(e.target.value)}><option value="">Any</option>{["A1", "A2", "B1", "B2", "C1", "C2"].map((value) => <option key={value}>{value}</option>)}</Select></Field>
          <div style={{ alignSelf: "end" }}><Button className="accent" onClick={search} disabled={busy}>{busy ? "Searching…" : "Search"}</Button></div>
        </div>
      </Card>
      <div className="section">
        {items.length === 0 ? <Empty>Search the dictionary to see lexemes.</Empty> : (
          <TableWrap>
            <Table>
              <TableHeader><TableRow><TableHead>Index</TableHead><TableHead>English</TableHead><TableHead>Uzbek</TableHead><TableHead>Level</TableHead><TableHead>POS</TableHead><TableHead>Source</TableHead></TableRow></TableHeader>
              <TableBody>{items.map((item) => (
                <TableRow key={item.index} onClick={() => void openSynonyms(item.index)} className="cursor-pointer">
                  <TableCell>{item.index}</TableCell><TableCell><b>{item.english}</b></TableCell><TableCell>{Array.isArray(item.uzbek) ? item.uzbek.join(", ") : String(item.uzbek)}</TableCell><TableCell><Pill>{item.cefr}</Pill></TableCell><TableCell>{item.part_of_speech || "—"}</TableCell><TableCell>{item.source_name}<div className="muted">{item.source_license}</div></TableCell>
                </TableRow>
              ))}</TableBody>
            </Table>
          </TableWrap>
        )}
      </div>
      {synonyms.length > 0 && <Card className="section"><h3>Synonyms</h3><div className="row" style={{ flexWrap: "wrap" }}>{synonyms.map((item) => <Pill key={item.word.index}>{item.word.english} · {Math.round((item.weight || 0) * 100)}%</Pill>)}</div></Card>}
    </>
  );
}
