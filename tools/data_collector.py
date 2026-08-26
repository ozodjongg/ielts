#!/usr/bin/env python3
"""
Stage 1: build a REAL English -> Uzbek lexical dataset from WordNet sources.

Sources:
- UzWordNet 1.1 (Uzbek, GWN-LMF with ILI)
- Open English WordNet 2025 (English, GWN-LMF with ILI)
- CEFR-J Vocabulary Profile 1.5
- Octanove C1/C2 Vocabulary Profile 1.0

Outputs:
- data/vocabulary/generated/stage1_core_all.csv
- data/vocabulary/generated/stage1_import_ready.csv
- data/vocabulary/generated/stage1_synonyms.csv
- data/vocabulary/generated/stage1_stats.txt

Important:
- No OPUS/sentence corpus is used.
- CEFR is NEVER invented. Entries without an official CEFR match remain only
  in stage1_core_all.csv and are not placed into stage1_import_ready.csv.
"""

from __future__ import annotations

import argparse
import csv
import gzip
import io
import os
import re
import shutil
import sys
import urllib.request
import xml.etree.ElementTree as ET
from collections import Counter, defaultdict
from pathlib import Path
from typing import Dict, Iterable, List, Optional, Set, Tuple

SOURCES = {
    "uzwordnet": (
        "https://raw.githubusercontent.com/LDKR-Group/UzWordnet/master/files/uzwordnet_ili.xml",
        "uzwordnet_ili.xml",
    ),
    "oewn": (
        "https://en-word.net/static/english-wordnet-2025.xml.gz",
        "english-wordnet-2025.xml.gz",
    ),
    "cefrj": (
        "https://raw.githubusercontent.com/vitwits/english-wordlist-cefr-a1-c2/master/cefrj-vocabulary-profile-1.5.csv",
        "cefrj-vocabulary-profile-1.5.csv",
    ),
    "octanove": (
        "https://raw.githubusercontent.com/vitwits/english-wordlist-cefr-a1-c2/master/octanove-vocabulary-profile-c1c2-1.0.csv",
        "octanove-vocabulary-profile-c1c2-1.0.csv",
    ),
}

CEFR_ORDER = {"A1": 1, "A2": 2, "B1": 3, "B2": 4, "C1": 5, "C2": 6}

POS_MAP = {
    "n": "noun",
    "noun": "noun",
    "v": "verb",
    "verb": "verb",
    "a": "adjective",
    "s": "adjective",
    "adj": "adjective",
    "adjective": "adjective",
    "r": "adverb",
    "adv": "adverb",
    "adverb": "adverb",
    "c": "conjunction",
    "conjunction": "conjunction",
    "p": "preposition",
    "prep": "preposition",
    "preposition": "preposition",
    "interjection": "interjection",
}

EN_ALLOWED = re.compile(r"^[A-Za-z]+(?:[ '\-][A-Za-z]+)*$")
# Uzbek Latin letters plus common apostrophe variants; no sentence punctuation.
UZ_ALLOWED = re.compile(
    r"^[A-Za-zÀ-ÖØ-öø-ÿʻʼ’‘`´'-]+(?:[ \-][A-Za-zÀ-ÖØ-öø-ÿʻʼ’‘`´'-]+)*$"
)


def local(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def norm_space(s: str) -> str:
    return re.sub(r"\s+", " ", s.strip())


def norm_english(s: str) -> str:
    return norm_space(s.replace("_", " "))


def norm_uzbek(s: str) -> str:
    s = norm_space(s.replace("_", " "))
    # normalize several apostrophe forms, but do not transliterate words
    s = s.replace("’", "'").replace("‘", "'").replace("ʼ", "'").replace("ʻ", "'")
    return s


def norm_pos(value: Optional[str]) -> str:
    if not value:
        return ""
    v = value.strip().lower()
    return POS_MAP.get(v, v)


def cefr_base(value: str) -> Optional[str]:
    m = re.search(r"\b(A1|A2|B1|B2|C1|C2)\b", value.upper())
    if m:
        return m.group(1)
    # CEFR-J sometimes has forms like A1.1/A1.2
    m = re.search(r"\b(A1|A2|B1|B2|C1|C2)(?:\.\d+)?", value.upper())
    return m.group(1) if m else None


def valid_english(s: str) -> bool:
    if not s or len(s) > 80:
        return False
    if len(s.split()) > 4:
        return False
    if any(ch.isdigit() for ch in s):
        return False
    if not EN_ALLOWED.fullmatch(s):
        return False
    # exclude suspicious all-uppercase abbreviations
    if len(s) > 1 and s.isupper():
        return False
    return True


def valid_uzbek(s: str) -> bool:
    if not s or len(s) > 120:
        return False
    if len(s.split()) > 6:
        return False
    if any(ch.isdigit() for ch in s):
        return False
    if any(ch in s for ch in ".!?;:%\"“”()[]{}<>/@\\=+*"):
        return False
    return bool(UZ_ALLOWED.fullmatch(s))


def download(url: str, dest: Path) -> None:
    if dest.exists() and dest.stat().st_size > 100:
        print(f"[OK] mavjud: {dest}")
        return

    dest.parent.mkdir(parents=True, exist_ok=True)
    tmp = dest.with_suffix(dest.suffix + ".part")
    print(f"[DL] {url}")
    req = urllib.request.Request(
        url,
        headers={"User-Agent": "assessment-platform-vocabulary-collector/1.0"},
    )
    with urllib.request.urlopen(req, timeout=120) as r, tmp.open("wb") as f:
        total = int(r.headers.get("Content-Length") or 0)
        got = 0
        while True:
            chunk = r.read(1024 * 1024)
            if not chunk:
                break
            f.write(chunk)
            got += len(chunk)
            if total:
                print(f"\r     {got/1024/1024:7.1f} / {total/1024/1024:7.1f} MiB", end="")
        if total:
            print()
    tmp.replace(dest)
    print(f"[OK] yuklandi: {dest}")


def open_xml(path: Path):
    if path.suffix.lower() == ".gz":
        return gzip.open(path, "rb")
    return path.open("rb")


def parse_wordnet(path: Path) -> Tuple[
    Dict[str, Set[Tuple[str, str]]],
    Dict[str, str],
    Dict[str, str],
]:
    """
    Returns:
      synset_lemmas: synset_id -> {(lemma, pos)}
      synset_ili:    synset_id -> ILI id
      synset_pos:    synset_id -> normalized POS
    """
    synset_lemmas: Dict[str, Set[Tuple[str, str]]] = defaultdict(set)
    synset_ili: Dict[str, str] = {}
    synset_pos: Dict[str, str] = {}

    print(f"[PARSE] {path}")
    with open_xml(path) as fh:
        for event, elem in ET.iterparse(fh, events=("end",)):
            name = local(elem.tag)

            if name == "LexicalEntry":
                lemma_text = ""
                lemma_pos = ""
                senses: List[str] = []

                # POS can occasionally exist on the entry itself
                entry_pos = norm_pos(
                    elem.attrib.get("partOfSpeech")
                    or elem.attrib.get("part_of_speech")
                    or elem.attrib.get("pos")
                )

                for child in elem.iter():
                    cname = local(child.tag)
                    if cname == "Lemma":
                        lemma_text = (
                            child.attrib.get("writtenForm")
                            or child.attrib.get("written_form")
                            or child.attrib.get("lemma")
                            or (child.text or "")
                        )
                        lemma_pos = norm_pos(
                            child.attrib.get("partOfSpeech")
                            or child.attrib.get("part_of_speech")
                            or child.attrib.get("pos")
                        )
                    elif cname == "Sense":
                        syn = (
                            child.attrib.get("synset")
                            or child.attrib.get("synsetId")
                            or child.attrib.get("synset_id")
                        )
                        if syn:
                            senses.append(syn)

                lemma_text = norm_space(lemma_text)
                pos = lemma_pos or entry_pos
                if lemma_text:
                    for syn in senses:
                        synset_lemmas[syn].add((lemma_text, pos))

                elem.clear()

            elif name == "Synset":
                sid = (
                    elem.attrib.get("id")
                    or elem.attrib.get("ID")
                    or elem.attrib.get("synset")
                )
                if sid:
                    ili = (
                        elem.attrib.get("ili")
                        or elem.attrib.get("ILI")
                        or elem.attrib.get("iliId")
                        or elem.attrib.get("ili_id")
                    )
                    pos = norm_pos(
                        elem.attrib.get("partOfSpeech")
                        or elem.attrib.get("part_of_speech")
                        or elem.attrib.get("pos")
                    )
                    if ili and ili not in {"in", "none", "null", "-"}:
                        synset_ili[sid] = ili
                    if pos:
                        synset_pos[sid] = pos
                elem.clear()

    print(
        f"[OK] synsets-with-lemmas={len(synset_lemmas):,}, "
        f"synsets-with-ILI={len(synset_ili):,}"
    )
    return synset_lemmas, synset_ili, synset_pos


def to_ili_map(
    synset_lemmas: Dict[str, Set[Tuple[str, str]]],
    synset_ili: Dict[str, str],
    synset_pos: Dict[str, str],
    language: str,
) -> Dict[str, Set[Tuple[str, str, str]]]:
    """ILI -> {(lemma, pos, synset_id)}"""
    out: Dict[str, Set[Tuple[str, str, str]]] = defaultdict(set)
    for sid, lemmas in synset_lemmas.items():
        ili = synset_ili.get(sid)
        if not ili:
            continue
        spos = synset_pos.get(sid, "")
        for lemma, lpos in lemmas:
            if language == "en":
                lemma = norm_english(lemma)
            else:
                lemma = norm_uzbek(lemma)
            out[ili].add((lemma, lpos or spos, sid))
    return out


def sniff_delimiter(path: Path) -> str:
    sample = path.read_text(encoding="utf-8-sig", errors="replace")[:8192]
    try:
        return csv.Sniffer().sniff(sample, delimiters=",;\t").delimiter
    except csv.Error:
        return ","


def load_cefr(paths: Iterable[Path]) -> Dict[Tuple[str, str], Tuple[str, str]]:
    """
    (normalized headword, normalized pos) -> (CEFR, source label)
    Also stores a no-POS fallback using pos="".
    """
    result: Dict[Tuple[str, str], Tuple[str, str]] = {}

    def put(key, level, source):
        old = result.get(key)
        if old is None or CEFR_ORDER[level] < CEFR_ORDER[old[0]]:
            result[key] = (level, source)

    for path in paths:
        if not path.exists():
            continue
        delim = sniff_delimiter(path)
        print(f"[CEFR] {path.name} delimiter={repr(delim)}")
        with path.open("r", encoding="utf-8-sig", errors="replace", newline="") as f:
            reader = csv.DictReader(f, delimiter=delim)
            if not reader.fieldnames:
                continue

            names = {n.strip().lower(): n for n in reader.fieldnames if n}
            word_col = (
                names.get("headword")
                or names.get("word")
                or names.get("lemma")
            )
            pos_col = names.get("pos") or names.get("part_of_speech")
            cefr_col = names.get("cefr") or names.get("level")
            if not word_col or not cefr_col:
                print(f"[WARN] CEFR ustunlari topilmadi: {reader.fieldnames}")
                continue

            label = "Octanove C1/C2" if "octanove" in path.name.lower() else "CEFR-J 1.5"
            for row in reader:
                word = norm_english(row.get(word_col, "")).lower()
                level = cefr_base(row.get(cefr_col, "") or "")
                if not word or not level:
                    continue
                pos = norm_pos(row.get(pos_col, "") if pos_col else "")
                put((word, pos), level, label)
                put((word, ""), level, label)

    print(f"[OK] CEFR mappings={len(result):,}")
    return result


def choose_cefr(
    cefr_map: Dict[Tuple[str, str], Tuple[str, str]],
    english: str,
    pos: str,
) -> Optional[Tuple[str, str]]:
    key = english.lower()
    return cefr_map.get((key, pos)) or cefr_map.get((key, ""))


def build(
    uz_path: Path,
    en_path: Path,
    cefr_paths: List[Path],
    out_dir: Path,
) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)

    uz_lemmas, uz_ili, uz_pos = parse_wordnet(uz_path)
    en_lemmas, en_ili, en_pos = parse_wordnet(en_path)

    uz_by_ili = to_ili_map(uz_lemmas, uz_ili, uz_pos, "uz")
    en_by_ili = to_ili_map(en_lemmas, en_ili, en_pos, "en")
    cefr_map = load_cefr(cefr_paths)

    common_ili = sorted(set(uz_by_ili) & set(en_by_ili))
    print(f"[JOIN] common ILI synsets={len(common_ili):,}")

    rows = {}
    synonym_edges: Set[Tuple[str, str]] = set()
    rejected = Counter()

    for ili in common_ili:
        uz_entries = uz_by_ili[ili]
        en_entries = en_by_ili[ili]

        clean_uz: List[str] = []
        for uz, _, _ in uz_entries:
            if valid_uzbek(uz):
                clean_uz.append(uz)
            else:
                rejected["uz_invalid"] += 1
        clean_uz = sorted(set(clean_uz), key=lambda s: (len(s), s.lower()))

        if not clean_uz:
            continue

        # English words in same synset = synonym candidates
        clean_en_for_syn = sorted({
            en.lower()
            for en, _, _ in en_entries
            if valid_english(norm_english(en))
        })
        for a in clean_en_for_syn:
            for b in clean_en_for_syn:
                if a != b:
                    synonym_edges.add((a, b))

        for en, pos, sid in en_entries:
            en = norm_english(en)
            if not valid_english(en):
                rejected["en_invalid"] += 1
                continue

            pos = norm_pos(pos) or norm_pos(en_pos.get(sid, ""))
            if pos not in {
                "noun", "verb", "adjective", "adverb",
                "conjunction", "preposition", "interjection"
            }:
                # keep a normalized lexical POS if available, otherwise skip
                if not pos:
                    rejected["missing_pos"] += 1
                    continue

            key = (en.lower(), pos)
            translations = clean_uz[:8]

            existing = rows.get(key)
            if existing:
                existing["uzbek"].update(translations)
                existing["ili"].add(ili)
            else:
                rows[key] = {
                    "english": en.lower(),
                    "uzbek": set(translations),
                    "part_of_speech": pos,
                    "ili": {ili},
                }

    all_path = out_dir / "stage1_core_all.csv"
    ready_path = out_dir / "stage1_import_ready.csv"
    syn_path = out_dir / "stage1_synonyms.csv"
    stats_path = out_dir / "stage1_stats.txt"

    all_fields = [
        "english", "uzbek", "part_of_speech", "cefr", "cefr_source",
        "source_name", "source_license", "source_ref"
    ]

    all_count = 0
    ready_count = 0
    pos_counts = Counter()
    cefr_counts = Counter()

    with all_path.open("w", encoding="utf-8", newline="") as fa, \
         ready_path.open("w", encoding="utf-8", newline="") as fr:

        wa = csv.DictWriter(fa, fieldnames=all_fields)
        wa.writeheader()

        # Importer-compatible fields only
        wr = csv.DictWriter(
            fr,
            fieldnames=[
                "english", "uzbek", "cefr",
                "source_name", "source_license",
                "part_of_speech", "source_ref"
            ],
        )
        wr.writeheader()

        for key in sorted(rows):
            item = rows[key]
            english = item["english"]
            pos = item["part_of_speech"]
            cefr = choose_cefr(cefr_map, english, pos)
            cefr_level = cefr[0] if cefr else ""
            cefr_source = cefr[1] if cefr else ""
            uzbek = "|".join(sorted(item["uzbek"], key=lambda s: (len(s), s.lower())))
            source_ref = "|".join(sorted(item["ili"]))

            source_name = "UzWordNet 1.1 + Open English WordNet 2025"
            source_license = "UzWordNet CC BY-SA 4.0; OEWN CC BY 4.0"

            row = {
                "english": english,
                "uzbek": uzbek,
                "part_of_speech": pos,
                "cefr": cefr_level,
                "cefr_source": cefr_source,
                "source_name": source_name,
                "source_license": source_license,
                "source_ref": source_ref,
            }
            wa.writerow(row)
            all_count += 1
            pos_counts[pos] += 1

            if cefr_level:
                ready_source_name = source_name + " + " + cefr_source
                ready_license = source_license
                if cefr_source == "CEFR-J 1.5":
                    ready_license += "; CEFR-J attribution terms"
                elif cefr_source == "Octanove C1/C2":
                    ready_license += "; Octanove CC BY-SA 4.0"

                wr.writerow({
                    "english": english,
                    "uzbek": uzbek,
                    "cefr": cefr_level,
                    "source_name": ready_source_name,
                    "source_license": ready_license,
                    "part_of_speech": pos,
                    "source_ref": source_ref,
                })
                ready_count += 1
                cefr_counts[cefr_level] += 1

    # Keep only synonym edges whose source and target are present in the lexical set.
    allowed_words = {k[0] for k in rows}
    kept_syn = sorted(
        (a, b) for a, b in synonym_edges
        if a in allowed_words and b in allowed_words
    )

    with syn_path.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=["english", "synonym", "source"])
        w.writeheader()
        for a, b in kept_syn:
            w.writerow({
                "english": a,
                "synonym": b,
                "source": "Open English WordNet 2025 synset",
            })

    stats = []
    stats.append("STAGE 1 VOCABULARY BUILD\n")
    stats.append(f"Common ILI synsets: {len(common_ili):,}\n")
    stats.append(f"Clean lexical rows (all): {all_count:,}\n")
    stats.append(f"Import-ready rows with official CEFR: {ready_count:,}\n")
    stats.append(f"Synonym edges: {len(kept_syn):,}\n")
    stats.append("\nPOS:\n")
    for k, v in pos_counts.most_common():
        stats.append(f"  {k}: {v:,}\n")
    stats.append("\nCEFR import-ready:\n")
    for level in CEFR_ORDER:
        stats.append(f"  {level}: {cefr_counts[level]:,}\n")
    stats.append("\nRejected:\n")
    for k, v in rejected.most_common():
        stats.append(f"  {k}: {v:,}\n")

    stats_path.write_text("".join(stats), encoding="utf-8")

    print("\n[DONE]")
    print(f"  all:        {all_path}")
    print(f"  import:     {ready_path}")
    print(f"  synonyms:   {syn_path}")
    print(f"  stats:      {stats_path}")
    print(f"  rows-all:   {all_count:,}")
    print(f"  CEFR-ready: {ready_count:,}")
    print(f"  synonyms:   {len(kept_syn):,}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--root",
        default=".",
        help="assessment platform project root (default: current directory)",
    )
    ap.add_argument(
        "--download-only",
        action="store_true",
        help="only download source files",
    )
    ap.add_argument(
        "--no-download",
        action="store_true",
        help="do not download; use already existing source files",
    )
    args = ap.parse_args()

    root = Path(args.root).resolve()
    source_dir = root / "data" / "vocabulary" / "sources"
    out_dir = root / "data" / "vocabulary" / "generated"

    paths = {}
    for name, (url, filename) in SOURCES.items():
        path = source_dir / filename
        paths[name] = path
        if not args.no_download:
            download(url, path)

    if args.download_only:
        print("[DONE] source files downloaded")
        return 0

    required = [paths["uzwordnet"], paths["oewn"], paths["cefrj"], paths["octanove"]]
    missing = [p for p in required if not p.exists()]
    if missing:
        print("[X] Missing source files:")
        for p in missing:
            print(f"  {p}")
        return 2

    build(
        uz_path=paths["uzwordnet"],
        en_path=paths["oewn"],
        cefr_paths=[paths["cefrj"], paths["octanove"]],
        out_dir=out_dir,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())