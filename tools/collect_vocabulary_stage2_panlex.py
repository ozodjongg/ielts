#!/usr/bin/env python3
"""
Stage 2: expand the English -> Uzbek lexical dataset with PanLex lexical meanings.

Input from Stage 1:
  data/vocabulary/generated/stage1_core_all.csv
  data/vocabulary/generated/stage1_import_ready.csv
  data/vocabulary/sources/cefrj-vocabulary-profile-1.5.csv
  data/vocabulary/sources/octanove-vocabulary-profile-c1c2-1.0.csv

Downloaded automatically:
  data/vocabulary/sources/panlex_eng.tsv
  data/vocabulary/sources/panlex_uzn.tsv

PanLex source:
  https://huggingface.co/datasets/cointegrated/panlex-meanings
  extracted from PanLex 20240301
  PanLex data license: CC0 1.0

Outputs:
  stage2_panlex_candidates.csv
  stage2_panlex_high_confidence.csv
  stage2_panlex_import_ready.csv
  stage2_merged_core_all.csv
  stage2_merged_import_ready.csv
  stage2_stats.txt

Design:
- joins English and Northern Uzbek expressions through PanLex meaning IDs
- uses lexical expressions only; no sentence corpus
- rejects punctuation-heavy, numeric, URL/code-like and long expressions
- keeps Uzbek Latin-script forms only
- does NOT invent CEFR levels
- high-confidence PanLex rows require at least one of:
    * English headword already exists in Stage 1 WordNet core
    * English headword has an official CEFR match
    * the exact EN-UZ pair is supported by >= 2 distinct PanLex meanings
"""

from __future__ import annotations

import argparse
import csv
import re
import sys
import urllib.request
from collections import Counter, defaultdict
from pathlib import Path
from typing import Dict, Iterable, List, Optional, Set, Tuple

PANLEX_URLS = {
    "eng": "https://huggingface.co/datasets/cointegrated/panlex-meanings/resolve/main/data/eng.tsv?download=true",
    "uzn": "https://huggingface.co/datasets/cointegrated/panlex-meanings/resolve/main/data/uzn.tsv?download=true",
}

CEFR_ORDER = {"A1": 1, "A2": 2, "B1": 3, "B2": 4, "C1": 5, "C2": 6}

EN_ALLOWED = re.compile(r"^[A-Za-z]+(?:[ '\-][A-Za-z]+)*$")
UZ_LATIN_ALLOWED = re.compile(
    r"^[A-Za-zÀ-ÖØ-öø-ÿʻʼ’‘`´'-]+(?:[ \-][A-Za-zÀ-ÖØ-öø-ÿʻʼ’‘`´'-]+)*$"
)

CYRILLIC_RE = re.compile(r"[\u0400-\u04FF]")
ARABIC_RE = re.compile(r"[\u0600-\u06FF]")

BAD_EN_TOKENS = {
    "http", "https", "www", "com", "org", "html", "xml", "json",
}
BAD_UZ_TOKENS = {"http", "https", "www"}

POS_FIELDS = ("part_of_speech", "pos")


def norm_space(s: str) -> str:
    return re.sub(r"\s+", " ", (s or "").strip())


def norm_en(s: str) -> str:
    return norm_space(s.replace("_", " ")).lower()


def norm_uz(s: str) -> str:
    s = norm_space(s.replace("_", " "))
    return (
        s.replace("’", "'")
         .replace("‘", "'")
         .replace("ʼ", "'")
         .replace("ʻ", "'")
    )


def valid_en(s: str) -> bool:
    if not s or len(s) > 80:
        return False
    if len(s.split()) > 4:
        return False
    if any(ch.isdigit() for ch in s):
        return False
    if not EN_ALLOWED.fullmatch(s):
        return False
    low = s.lower()
    if any(tok in low.split() for tok in BAD_EN_TOKENS):
        return False
    if len(s) > 1 and s.isupper():
        return False
    return True


def valid_uz_latin(s: str) -> bool:
    if not s or len(s) > 100:
        return False
    if len(s.split()) > 5:
        return False
    if any(ch.isdigit() for ch in s):
        return False
    if CYRILLIC_RE.search(s) or ARABIC_RE.search(s):
        return False
    if any(ch in s for ch in ".!?;:%\"“”()[]{}<>/@\\=+*"):
        return False
    if not UZ_LATIN_ALLOWED.fullmatch(s):
        return False
    low = s.lower()
    if any(tok in low.split() for tok in BAD_UZ_TOKENS):
        return False
    return True


def download(url: str, dest: Path) -> None:
    if dest.exists() and dest.stat().st_size > 100:
        print(f"[OK] mavjud: {dest} ({dest.stat().st_size/1024/1024:.1f} MiB)")
        return

    dest.parent.mkdir(parents=True, exist_ok=True)
    tmp = dest.with_suffix(dest.suffix + ".part")
    req = urllib.request.Request(
        url,
        headers={"User-Agent": "assessment-platform-vocabulary-collector/2.0"},
    )

    print(f"[DL] {url}")
    with urllib.request.urlopen(req, timeout=180) as r, tmp.open("wb") as f:
        total = int(r.headers.get("Content-Length") or 0)
        got = 0
        while True:
            chunk = r.read(1024 * 1024)
            if not chunk:
                break
            f.write(chunk)
            got += len(chunk)
            if total:
                print(
                    f"\r     {got/1024/1024:8.1f} / {total/1024/1024:8.1f} MiB "
                    f"({100*got/total:5.1f}%)",
                    end="",
                    flush=True,
                )
            else:
                print(f"\r     {got/1024/1024:8.1f} MiB", end="", flush=True)
        print()

    tmp.replace(dest)
    print(f"[OK] yuklandi: {dest}")


def sniff_delimiter(path: Path) -> str:
    with path.open("r", encoding="utf-8-sig", errors="replace") as f:
        sample = f.read(8192)
    try:
        return csv.Sniffer().sniff(sample, delimiters=",;\t").delimiter
    except csv.Error:
        return ","


def cefr_base(value: str) -> Optional[str]:
    m = re.search(r"\b(A1|A2|B1|B2|C1|C2)(?:\.\d+)?\b", (value or "").upper())
    return m.group(1) if m else None


def load_cefr(paths: Iterable[Path]) -> Dict[str, Tuple[str, str]]:
    """English headword -> (lowest official CEFR level, source label)."""
    out: Dict[str, Tuple[str, str]] = {}

    for path in paths:
        if not path.exists():
            continue
        delim = sniff_delimiter(path)
        with path.open("r", encoding="utf-8-sig", errors="replace", newline="") as f:
            reader = csv.DictReader(f, delimiter=delim)
            if not reader.fieldnames:
                continue
            names = {n.strip().lower(): n for n in reader.fieldnames if n}
            word_col = names.get("headword") or names.get("word") or names.get("lemma")
            level_col = names.get("cefr") or names.get("level")
            if not word_col or not level_col:
                continue

            label = (
                "Octanove C1/C2"
                if "octanove" in path.name.lower()
                else "CEFR-J 1.5"
            )

            for row in reader:
                word = norm_en(row.get(word_col, ""))
                level = cefr_base(row.get(level_col, ""))
                if not word or not level:
                    continue
                old = out.get(word)
                if old is None or CEFR_ORDER[level] < CEFR_ORDER[old[0]]:
                    out[word] = (level, label)

    print(f"[OK] official CEFR headwords: {len(out):,}")
    return out


def load_stage1_words(path: Path) -> Tuple[Set[str], Dict[str, Set[str]], Dict[str, str]]:
    """
    Returns:
      known English headwords
      English -> known POS values
      English -> joined existing Uzbek translations
    """
    words: Set[str] = set()
    pos_map: Dict[str, Set[str]] = defaultdict(set)
    uz_map: Dict[str, Set[str]] = defaultdict(set)

    if not path.exists():
        raise FileNotFoundError(f"Stage 1 file topilmadi: {path}")

    with path.open("r", encoding="utf-8-sig", newline="") as f:
        reader = csv.DictReader(f)
        for row in reader:
            en = norm_en(row.get("english", ""))
            if not en:
                continue
            words.add(en)
            pos = norm_space(row.get("part_of_speech", ""))
            if pos:
                pos_map[en].add(pos)
            for uz in (row.get("uzbek", "") or "").split("|"):
                uz = norm_uz(uz)
                if uz:
                    uz_map[en].add(uz)

    print(f"[OK] Stage 1 English headwords: {len(words):,}")
    return words, pos_map, uz_map


def read_tsv(path: Path):
    with path.open("r", encoding="utf-8-sig", errors="replace", newline="") as f:
        reader = csv.DictReader(f, delimiter="\t")
        required = {"txt", "meaning", "langvar_uid"}
        missing = required - set(reader.fieldnames or [])
        if missing:
            raise RuntimeError(
                f"{path.name}: kerakli ustunlar yo'q: {sorted(missing)}; "
                f"bor: {reader.fieldnames}"
            )
        for row in reader:
            yield row


def build_panlex_pairs(
    eng_path: Path,
    uz_path: Path,
    stage1_words: Set[str],
    cefr_map: Dict[str, Tuple[str, str]],
) -> Tuple[
    Dict[Tuple[str, str], Set[int]],
    Counter,
]:
    """
    Returns exact EN-UZ pair -> set of PanLex meaning IDs supporting it.
    """
    stats = Counter()

    # Meaning ID -> Uzbek Latin expressions
    uz_by_meaning: Dict[int, Set[str]] = defaultdict(set)

    print("[PARSE] PanLex Uzbek...")
    for row in read_tsv(uz_path):
        stats["uz_rows_total"] += 1
        lv = row.get("langvar_uid", "")
        if not lv.startswith("uzn-"):
            stats["uz_wrong_variety"] += 1
            continue

        uz = norm_uz(row.get("txt", ""))
        if not valid_uz_latin(uz):
            stats["uz_rejected"] += 1
            continue

        try:
            meaning = int(row.get("meaning", ""))
        except ValueError:
            stats["uz_bad_meaning"] += 1
            continue

        uz_by_meaning[meaning].add(uz)
        stats["uz_rows_kept"] += 1

    print(
        f"[OK] Uzbek meanings: {len(uz_by_meaning):,}; "
        f"kept rows: {stats['uz_rows_kept']:,}"
    )

    pair_support: Dict[Tuple[str, str], Set[int]] = defaultdict(set)

    print("[PARSE] PanLex English + join...")
    for row in read_tsv(eng_path):
        stats["en_rows_total"] += 1
        try:
            meaning = int(row.get("meaning", ""))
        except ValueError:
            stats["en_bad_meaning"] += 1
            continue

        uz_values = uz_by_meaning.get(meaning)
        if not uz_values:
            continue

        lv = row.get("langvar_uid", "")
        # Accept English varieties, but lexical validation below is strict.
        if not lv.startswith("eng-"):
            continue

        en = norm_en(row.get("txt", ""))
        if not valid_en(en):
            stats["en_rejected"] += 1
            continue

        for uz in uz_values:
            # reject accidental identical strings unless likely loanword/cognate;
            # keep them as candidates only if Stage 1 or CEFR knows the English word.
            if en.casefold() == uz.casefold() and en not in stage1_words and en not in cefr_map:
                stats["identical_unverified"] += 1
                continue
            pair_support[(en, uz)].add(meaning)
            stats["pair_links"] += 1

    print(f"[OK] distinct EN-UZ pairs: {len(pair_support):,}")
    return pair_support, stats


def choose_pos(pos_map: Dict[str, Set[str]], en: str) -> str:
    vals = sorted(pos_map.get(en, set()))
    if len(vals) == 1:
        return vals[0]
    return ""


def write_outputs(
    out_dir: Path,
    pair_support: Dict[Tuple[str, str], Set[int]],
    stats: Counter,
    stage1_all: Path,
    stage1_ready: Path,
    stage1_words: Set[str],
    stage1_pos: Dict[str, Set[str]],
    cefr_map: Dict[str, Tuple[str, str]],
) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)

    candidates_path = out_dir / "stage2_panlex_candidates.csv"
    high_path = out_dir / "stage2_panlex_high_confidence.csv"
    ready_path = out_dir / "stage2_panlex_import_ready.csv"
    merged_all_path = out_dir / "stage2_merged_core_all.csv"
    merged_ready_path = out_dir / "stage2_merged_import_ready.csv"
    stats_path = out_dir / "stage2_stats.txt"

    # Aggregate translations by English headword for import-like files.
    all_by_en: Dict[str, Set[str]] = defaultdict(set)
    high_by_en: Dict[str, Set[str]] = defaultdict(set)
    support_max: Dict[str, int] = defaultdict(int)
    high_reason: Dict[str, Set[str]] = defaultdict(set)

    with candidates_path.open("w", encoding="utf-8", newline="") as fc, \
         high_path.open("w", encoding="utf-8", newline="") as fh:

        fields = [
            "english", "uzbek", "support_count", "meaning_ids",
            "known_in_stage1", "official_cefr", "confidence_reason",
            "source_name", "source_license"
        ]
        wc = csv.DictWriter(fc, fieldnames=fields)
        wh = csv.DictWriter(fh, fieldnames=fields)
        wc.writeheader()
        wh.writeheader()

        for (en, uz), meanings in sorted(pair_support.items()):
            support = len(meanings)
            all_by_en[en].add(uz)
            support_max[en] = max(support_max[en], support)

            reasons = []
            if en in stage1_words:
                reasons.append("stage1_wordnet")
            if en in cefr_map:
                reasons.append("official_cefr")
            if support >= 2:
                reasons.append("panlex_multi_meaning_support")

            row = {
                "english": en,
                "uzbek": uz,
                "support_count": support,
                "meaning_ids": "|".join(str(x) for x in sorted(meanings)[:20]),
                "known_in_stage1": "1" if en in stage1_words else "0",
                "official_cefr": cefr_map.get(en, ("", ""))[0],
                "confidence_reason": "|".join(reasons),
                "source_name": "PanLex 20240301 via cointegrated/panlex-meanings",
                "source_license": "CC0 1.0",
            }
            wc.writerow(row)

            if reasons:
                wh.writerow(row)
                high_by_en[en].add(uz)
                high_reason[en].update(reasons)

    # PanLex import-ready: official CEFR only, no invented level.
    with ready_path.open("w", encoding="utf-8", newline="") as f:
        fields = [
            "english", "uzbek", "cefr", "source_name", "source_license",
            "part_of_speech", "source_ref"
        ]
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        ready_count = 0

        for en in sorted(high_by_en):
            cefr = cefr_map.get(en)
            if not cefr:
                continue
            level, level_source = cefr
            uzbek = "|".join(sorted(high_by_en[en], key=lambda x: (len(x), x.lower()))[:12])
            w.writerow({
                "english": en,
                "uzbek": uzbek,
                "cefr": level,
                "source_name": (
                    "PanLex 20240301 + " + level_source
                ),
                "source_license": "PanLex CC0 1.0; CEFR source attribution required",
                "part_of_speech": choose_pos(stage1_pos, en),
                "source_ref": "confidence=" + "|".join(sorted(high_reason[en])),
            })
            ready_count += 1

    # Merge Stage 1 ALL + PanLex high-confidence candidates.
    # This audit file may contain blank CEFR for PanLex-only words.
    with merged_all_path.open("w", encoding="utf-8", newline="") as fout:
        fields = [
            "english", "uzbek", "part_of_speech", "cefr", "cefr_source",
            "source_name", "source_license", "source_ref"
        ]
        w = csv.DictWriter(fout, fieldnames=fields)
        w.writeheader()

        seen: Set[Tuple[str, str]] = set()

        with stage1_all.open("r", encoding="utf-8-sig", newline="") as f:
            r = csv.DictReader(f)
            for row in r:
                key = (norm_en(row.get("english", "")), row.get("part_of_speech", ""))
                if not key[0]:
                    continue
                w.writerow({k: row.get(k, "") for k in fields})
                seen.add(key)

        for en in sorted(high_by_en):
            pos = choose_pos(stage1_pos, en)
            key = (en, pos)
            if key in seen:
                # Stage 1 entry already exists; we do not duplicate it in core_all.
                continue
            cefr = cefr_map.get(en, ("", ""))
            w.writerow({
                "english": en,
                "uzbek": "|".join(sorted(high_by_en[en], key=lambda x: (len(x), x.lower()))[:12]),
                "part_of_speech": pos,
                "cefr": cefr[0],
                "cefr_source": cefr[1],
                "source_name": "PanLex 20240301",
                "source_license": "CC0 1.0",
                "source_ref": "confidence=" + "|".join(sorted(high_reason[en])),
            })
            seen.add(key)

    # Merge importer-compatible Stage 1 + PanLex official-CEFR rows.
    # Deduplicate by English + POS + source_name semantics without altering Stage 1.
    with merged_ready_path.open("w", encoding="utf-8", newline="") as fout:
        fields = [
            "english", "uzbek", "cefr",
            "source_name", "source_license",
            "part_of_speech", "source_ref"
        ]
        w = csv.DictWriter(fout, fieldnames=fields)
        w.writeheader()

        stage1_ready_words: Set[str] = set()

        with stage1_ready.open("r", encoding="utf-8-sig", newline="") as f:
            r = csv.DictReader(f)
            for row in r:
                w.writerow({k: row.get(k, "") for k in fields})
                stage1_ready_words.add(norm_en(row.get("english", "")))

        with ready_path.open("r", encoding="utf-8-sig", newline="") as f:
            r = csv.DictReader(f)
            for row in r:
                en = norm_en(row.get("english", ""))
                # If Stage 1 already has this word, its WordNet translation is the primary row.
                # PanLex remains in the audit/high-confidence files instead of creating a duplicate.
                if en in stage1_ready_words:
                    continue
                w.writerow({k: row.get(k, "") for k in fields})

    high_pair_count = sum(
        1
        for (en, _uz), meanings in pair_support.items()
        if en in stage1_words or en in cefr_map or len(meanings) >= 2
    )
    high_word_count = len(high_by_en)
    candidate_word_count = len(all_by_en)

    lines = [
        "STAGE 2 PANLEX BUILD\n",
        f"Candidate distinct EN-UZ pairs: {len(pair_support):,}\n",
        f"Candidate English headwords: {candidate_word_count:,}\n",
        f"High-confidence EN-UZ pairs: {high_pair_count:,}\n",
        f"High-confidence English headwords: {high_word_count:,}\n",
        f"PanLex official-CEFR import-ready headwords: {ready_count:,}\n",
        "\nParser counters:\n",
    ]
    for k, v in stats.most_common():
        lines.append(f"  {k}: {v:,}\n")
    stats_path.write_text("".join(lines), encoding="utf-8")

    print("\n[DONE]")
    print(f"  candidates:   {candidates_path}")
    print(f"  high-conf:    {high_path}")
    print(f"  import-ready: {ready_path}")
    print(f"  merged-all:   {merged_all_path}")
    print(f"  merged-ready: {merged_ready_path}")
    print(f"  stats:        {stats_path}")
    print(f"  candidate EN-UZ pairs: {len(pair_support):,}")
    print(f"  high-confidence words: {high_word_count:,}")
    print(f"  CEFR-ready PanLex words: {ready_count:,}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=".", help="project root")
    ap.add_argument("--no-download", action="store_true")
    args = ap.parse_args()

    root = Path(args.root).resolve()
    src_dir = root / "data" / "vocabulary" / "sources"
    out_dir = root / "data" / "vocabulary" / "generated"

    stage1_all = out_dir / "stage1_core_all.csv"
    stage1_ready = out_dir / "stage1_import_ready.csv"
    cefrj = src_dir / "cefrj-vocabulary-profile-1.5.csv"
    octanove = src_dir / "octanove-vocabulary-profile-c1c2-1.0.csv"

    eng_path = src_dir / "panlex_eng.tsv"
    uz_path = src_dir / "panlex_uzn.tsv"

    if not args.no_download:
        download(PANLEX_URLS["uzn"], uz_path)
        download(PANLEX_URLS["eng"], eng_path)

    for p in (stage1_all, stage1_ready, cefrj, octanove, eng_path, uz_path):
        if not p.exists():
            print(f"[X] file topilmadi: {p}")
            return 2

    stage1_words, stage1_pos, _stage1_uz = load_stage1_words(stage1_all)
    cefr_map = load_cefr([cefrj, octanove])

    pair_support, stats = build_panlex_pairs(
        eng_path=eng_path,
        uz_path=uz_path,
        stage1_words=stage1_words,
        cefr_map=cefr_map,
    )

    write_outputs(
        out_dir=out_dir,
        pair_support=pair_support,
        stats=stats,
        stage1_all=stage1_all,
        stage1_ready=stage1_ready,
        stage1_words=stage1_words,
        stage1_pos=stage1_pos,
        cefr_map=cefr_map,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
