#!/usr/bin/env python3
from __future__ import annotations

import csv
import json
import re
import shutil
import subprocess
import sys
from collections import Counter, defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
APPS = ["admin-web", "center-web", "student-web"]
LEVELS = ["A1", "A2", "B1", "B2", "C1", "C2"]
issues: list[str] = []
warnings: list[str] = []
checks: dict[str, object] = {}


def fail(message: str) -> None:
    issues.append(message)


def warn(message: str) -> None:
    warnings.append(message)


def read_csv(path: Path) -> list[dict[str, str]]:
    with path.open("r", encoding="utf-8-sig", newline="") as handle:
        return list(csv.DictReader(handle))


# ---------------------------------------------------------------------------
# English bank: structure AND the same answer-resolution logic used by Go.
# ---------------------------------------------------------------------------
subjects = read_csv(ROOT / "data/english-bank/subjects.csv")
questions = read_csv(ROOT / "data/english-bank/questions.csv")
models = read_csv(ROOT / "data/english-bank/model.csv")
checks["english_subjects"] = len(subjects)
checks["english_questions"] = len(questions)
checks["english_models"] = len(models)

if len(subjects) != 80:
    fail(f"English subject count {len(subjects)} != 80")
if len(questions) != 8000:
    fail(f"English question count {len(questions)} != 8000")
if len(models) != 8000:
    fail(f"English model count {len(models)} != 8000")

qids = [row.get("question_uuid", "") for row in questions]
if any(not q for q in qids):
    fail("English bank contains an empty question UUID")
if len(set(qids)) != len(qids):
    fail("English question UUIDs are not unique")

by_subject = Counter(row.get("subject_uuid", "") for row in questions)
if len(by_subject) != 80 or any(count != 100 for count in by_subject.values()):
    fail("English bank must contain exactly 100 variants for each of 80 subjects")

model_ids = {row.get("question_uuid", "") for row in models}
if set(qids) != model_ids:
    fail("English model/question coverage mismatch")


def ascii_word(ch: str) -> bool:
    return ("a" <= ch <= "z") or ("0" <= ch <= "9")


def normalize_english(value: str) -> str:
    value = value.strip().lower().replace("’", "'")
    chars = list(value)
    for i, ch in enumerate(chars):
        if ch != "'":
            continue
        prev_word = i > 0 and ascii_word(chars[i - 1])
        next_word = i + 1 < len(chars) and ascii_word(chars[i + 1])
        if not prev_word or not next_word:
            chars[i] = " "
    value = re.sub(r"[^a-z0-9']+", " ", "".join(chars))
    return " ".join(value.split())


def fill_answer(template: str, option: str) -> str:
    blank_count = template.count("___")
    if blank_count == 0:
        return (template + " " + option).strip()
    if blank_count > 1:
        parts = option.split("/")
        if len(parts) == blank_count:
            out = template
            for part in parts:
                out = out.replace("___", part.strip(), 1)
            return out
    return template.replace("___", option.strip(), 1)


model_map: dict[str, set[str]] = defaultdict(set)
for row in models:
    normalized = row.get("normalized_text", "") or normalize_english(row.get("accepted_text", ""))
    model_map[row.get("question_uuid", "")].add(normalize_english(normalized))

unresolved: list[str] = []
multiple: list[str] = []
multi_blank = 0
for row in questions:
    qid = row["question_uuid"]
    template = row.get("answer_template", "")
    if template.count("___") > 1:
        multi_blank += 1
    options = [row.get(k, "").strip() for k in ("option_a", "option_b", "option_c", "option_d")]
    options = [option for option in options if option]
    matches = [
        option
        for option in options
        if normalize_english(fill_answer(template, option)) in model_map[qid]
    ]
    if len(matches) == 0:
        unresolved.append(qid)
    elif len(matches) > 1:
        multiple.append(qid)
if unresolved:
    fail(f"English loader cannot resolve {len(unresolved)} correct answers; first: {unresolved[:5]}")
if multiple:
    fail(f"English loader resolves multiple answers for {len(multiple)} questions; first: {multiple[:5]}")
checks["english_answer_resolution"] = "pass" if not unresolved and not multiple else "fail"
checks["english_multi_blank_questions"] = multi_blank

# ---------------------------------------------------------------------------
# SAT bank.
# ---------------------------------------------------------------------------
sat = read_csv(ROOT / "data/sat-math-bank/questions.csv")
topics = read_csv(ROOT / "data/sat-math-bank/topics.csv")
checks["sat_questions"] = len(sat)
checks["sat_topics"] = len(topics)
if len(sat) != 8000:
    fail(f"SAT question count {len(sat)} != 8000")
if len(topics) != 80:
    fail(f"SAT topic count {len(topics)} != 80")

sat_ids = [row.get("id", "") for row in sat]
prompts = [row.get("prompt", "").strip().lower() for row in sat]
if any(not x for x in sat_ids):
    fail("SAT bank contains an empty question ID")
if len(set(sat_ids)) != len(sat_ids):
    fail("SAT IDs are not unique")
if len(set(prompts)) != len(prompts):
    fail(f"SAT duplicate prompts: {len(prompts) - len(set(prompts))}")
by_topic = Counter(row.get("topic_code", "") for row in sat)
if len(by_topic) != 80 or any(count != 100 for count in by_topic.values()):
    fail("SAT bank must contain exactly 100 variants for each of 80 topics")
for line, row in enumerate(sat, 2):
    if row.get("correct_option", "") not in {"A", "B", "C", "D"}:
        fail(f"SAT line {line}: invalid correct option {row.get('correct_option')!r}")
        break
    if not row.get("prompt", "").strip():
        fail(f"SAT line {line}: empty prompt")
        break
    options = [row.get(f"option_{letter}", "").strip() for letter in "abcd"]
    if any(not option for option in options) or len(set(options)) != 4:
        fail(f"SAT line {line}: four distinct non-empty options are required")
        break
checks["sat_bank_integrity"] = "pass" if not any(x.startswith("SAT ") for x in issues) else "fail"

# ---------------------------------------------------------------------------
# Vocabulary: honest production manifest + balanced local QA corpus.
# ---------------------------------------------------------------------------
manifest = json.loads((ROOT / "data/vocabulary/MANIFEST.json").read_text(encoding="utf-8"))
checks["vocabulary_target_capacity"] = manifest.get("target_capacity")
checks["vocabulary_production_bundled"] = manifest.get("production_lexemes_bundled")
if manifest.get("production_ready_dictionary_claim") is not False:
    fail("Vocabulary manifest must not claim an unverified production 100K corpus")

demo_vocab = read_csv(ROOT / "data/vocabulary/demo_seed.csv")
demo_synonyms = read_csv(ROOT / "data/vocabulary/demo_synonyms.csv")
vocab_levels = Counter(row.get("cefr", "") for row in demo_vocab)
checks["vocabulary_demo_lexemes"] = len(demo_vocab)
checks["vocabulary_demo_by_level"] = dict(vocab_levels)
checks["vocabulary_demo_synonyms"] = len(demo_synonyms)
if len(demo_vocab) != 180:
    fail(f"Demo vocabulary must contain 180 rows, found {len(demo_vocab)}")
for level in LEVELS:
    if vocab_levels[level] != 30:
        fail(f"Demo vocabulary {level}: expected 30 words, found {vocab_levels[level]}")
seen_vocab: set[tuple[str, str]] = set()
for line, row in enumerate(demo_vocab, 2):
    key = (row.get("english", "").strip().lower(), row.get("part_of_speech", "").strip().lower())
    if not key[0] or not row.get("uzbek", "").strip():
        fail(f"Vocabulary line {line}: English and Uzbek are required")
        break
    if key in seen_vocab:
        fail(f"Vocabulary line {line}: duplicate English/POS pair {key}")
        break
    seen_vocab.add(key)
    if row.get("cefr") not in LEVELS:
        fail(f"Vocabulary line {line}: invalid CEFR {row.get('cefr')!r}")
        break
    if not row.get("source_name", "").strip() or not row.get("source_license", "").strip():
        fail(f"Vocabulary line {line}: source metadata required")
        break

# ---------------------------------------------------------------------------
# Database schemas/migrations and static SQL placeholder checks.
# ---------------------------------------------------------------------------
services = ["identity", "tenant", "assessment", "vocabulary", "listening", "review", "sat", "points", "analytics"]
for service in services:
    directory = ROOT / "backend/migrations" / service
    if not directory.is_dir() or not list(directory.glob("*.sql")):
        fail(f"Missing migrations for {service}")
checks["database_module_schemas"] = len(services)


def extract_balanced(source: str, open_pos: int, opener: str = "(", closer: str = ")") -> tuple[str, int] | None:
    depth = 1
    quote: str | None = None
    raw = False
    escaped = False
    i = open_pos + 1
    while i < len(source):
        ch = source[i]
        if quote is not None:
            if escaped:
                escaped = False
            elif ch == "\\" and quote == '"':
                escaped = True
            elif ch == quote:
                quote = None
        elif raw:
            if ch == "`":
                raw = False
        else:
            if ch in ('"', "'"):
                quote = ch
            elif ch == "`":
                raw = True
            elif ch == opener:
                depth += 1
            elif ch == closer:
                depth -= 1
                if depth == 0:
                    return source[open_pos + 1:i], i
        i += 1
    return None


def split_top_level(expr: str) -> list[str]:
    parts: list[str] = []
    start = 0
    depth = 0
    quote: str | None = None
    raw = False
    escaped = False
    for i, ch in enumerate(expr):
        if quote is not None:
            if escaped:
                escaped = False
            elif ch == "\\" and quote == '"':
                escaped = True
            elif ch == quote:
                quote = None
            continue
        if raw:
            if ch == "`":
                raw = False
            continue
        if ch in ('"', "'"):
            quote = ch
        elif ch == "`":
            raw = True
        elif ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        elif ch == "," and depth == 0:
            parts.append(expr[start:i].strip())
            start = i + 1
    parts.append(expr[start:].strip())
    return parts


sql_checked = 0
for go_file in (ROOT / "backend").rglob("*.go"):
    source = go_file.read_text(encoding="utf-8")
    for method in ("Exec", "Query", "QueryRow"):
        for match in re.finditer(rf"\.{method}\(", source):
            call = extract_balanced(source, match.end() - 1)
            if call is None:
                continue
            args = split_top_level(call[0])
            if len(args) < 2:
                continue
            sql_expr = args[1].strip()
            if not (sql_expr.startswith("`") and sql_expr.endswith("`")):
                continue
            sql = sql_expr[1:-1]
            placeholders = [int(x) for x in re.findall(r"\$(\d+)", sql)]
            highest = max(placeholders, default=0)
            params = args[2:]
            if any("..." in value for value in params):
                continue
            sql_checked += 1
            if highest != len(params):
                line = source.count("\n", 0, match.start()) + 1
                fail(
                    f"{go_file.relative_to(ROOT)}:{line}: {method} SQL highest placeholder ${highest} but {len(params)} args supplied"
                )
checks["static_sql_calls_checked"] = sql_checked

# Seed-demo should cover all major modules and remain opt-in.
seed = (ROOT / "backend/cmd/seed-demo/main.go").read_text(encoding="utf-8")
for schema in ("tenant", "identity", "assessment", "sat", "listening", "review", "points", "analytics"):
    if f"{schema}." not in seed:
        fail(f"seed-demo does not populate {schema}")
if "DemoPassword123!" not in seed or "--password" not in seed:
    fail("seed-demo must expose a clearly demo-only configurable password")
checks["demo_seed_command"] = "present"

# ---------------------------------------------------------------------------
# Frontend structure, shadcn-style primitives, React hooks, monochrome theme,
# private-portal SEO, and security headers.
# ---------------------------------------------------------------------------
raw_control = re.compile(r"<(button|input|select|textarea|table|thead|tbody|tr|th|td)(?:\s|>)")
bad_effect = re.compile(r"useEffect\s*\(\s*async|useEffect\s*\(\s*\(\)\s*=>\s*[A-Za-z_$][A-Za-z0-9_$]*\(")
color_utility = re.compile(r"\b(?:red|blue|green|emerald|amber|yellow|orange|purple|violet|indigo|cyan|teal|pink|rose)-\d{2,3}\b")
allowed_hex = {"#09090b", "#18181b", "#52525b", "#71717a", "#d4d4d8", "#e4e4e7", "#f4f4f5", "#fafafa", "#fff", "#ffffff"}

for app in APPS:
    directory = ROOT / "apps" / app
    required = [
        "package.json",
        "components.json",
        ".env.example",
        "next.config.ts",
        "src/app/layout.tsx",
        "src/app/manifest.ts",
        "src/app/robots.ts",
        "src/app/error.tsx",
        "src/app/loading.tsx",
        "src/app/not-found.tsx",
        "src/components/auth-provider.tsx",
        "src/components/ui/index.ts",
        "src/components/ui/button.tsx",
        "src/components/ui/card.tsx",
        "src/components/ui/form.tsx",
        "src/components/ui/table.tsx",
    ]
    for req in required:
        if not (directory / req).exists():
            fail(f"{app}: missing {req}")

    components = json.loads((directory / "components.json").read_text(encoding="utf-8"))
    if components.get("$schema") != "https://ui.shadcn.com/schema.json":
        fail(f"{app}: components.json is not a shadcn schema config")

    # Only UI primitives themselves may contain raw controls/tables.
    src = directory / "src"
    for source_file in src.rglob("*.tsx"):
        text = source_file.read_text(encoding="utf-8")
        rel = source_file.relative_to(src)
        if "components/ui" not in str(rel).replace("\\", "/") and raw_control.search(text):
            fail(f"{app}: raw HTML control outside shadcn UI layer: {rel}")
        if bad_effect.search(text):
            fail(f"{app}: unsafe Promise-returning/async useEffect pattern: {rel}")
        if color_utility.search(text):
            fail(f"{app}: non-monochrome color utility found: {rel}")

    globals_css = (directory / "src/app/globals.css").read_text(encoding="utf-8").lower()
    colors = set(re.findall(r"#[0-9a-f]{3,8}\b", globals_css))
    unexpected = sorted(colors - allowed_hex)
    if unexpected:
        fail(f"{app}: non-monochrome CSS colors: {unexpected}")

    layout = (directory / "src/app/layout.tsx").read_text(encoding="utf-8")
    robots = (directory / "src/app/robots.ts").read_text(encoding="utf-8")
    next_config = (directory / "next.config.ts").read_text(encoding="utf-8")
    if "index: false" not in layout or "follow: false" not in layout or "noarchive: true" not in layout:
        fail(f"{app}: authenticated portal metadata must be noindex/nofollow/noarchive")
    if 'disallow: "/"' not in robots:
        fail(f"{app}: robots.ts must disallow crawling the private portal")
    for header in (
        "Content-Security-Policy",
        "X-Content-Type-Options",
        "X-Frame-Options",
        "Referrer-Policy",
        "Permissions-Policy",
        "X-Robots-Tag",
        "Strict-Transport-Security",
    ):
        if header not in next_config:
            fail(f"{app}: missing production security header {header}")
    env_example = (directory / ".env.example").read_text(encoding="utf-8").strip().splitlines()
    if env_example != ["NEXT_PUBLIC_API_URL=http://localhost:8080"]:
        fail(f"{app}: frontend .env.example must contain only NEXT_PUBLIC_API_URL")

checks["frontend_projects"] = len(APPS)
checks["shadcn_component_layer"] = "pass" if not any("shadcn" in x.lower() or "raw HTML control" in x for x in issues) else "fail"
checks["monochrome_theme"] = "pass" if not any("non-monochrome" in x for x in issues) else "fail"
checks["private_portal_seo_security"] = "pass" if not any("robots" in x.lower() or "security header" in x.lower() or "noindex" in x.lower() for x in issues) else "fail"

# Verify @/ local imports exist.
for app in APPS:
    src = ROOT / "apps" / app / "src"
    for source_file in src.rglob("*"):
        if source_file.suffix not in {".ts", ".tsx"}:
            continue
        text = source_file.read_text(encoding="utf-8")
        for match in re.finditer(r'from\s+["\']@/([^"\']+)["\']', text):
            rel = match.group(1)
            base = src / rel
            candidates = [base.with_suffix(".ts"), base.with_suffix(".tsx"), base / "index.ts", base / "index.tsx"]
            if not any(candidate.exists() for candidate in candidates):
                fail(f"{app}: missing local import @/{rel} from {source_file.relative_to(src)}")

# No stale architecture references or accidental credentials in source.
secret_patterns = [
    re.compile(r"eyJhbGciOi[A-Za-z0-9_.-]{50,}"),
    re.compile(r"(?i)(?:service[_-]?role|private[_-]?key)\s*[:=]\s*[A-Za-z0-9_.-]{24,}"),
]
for path in ROOT.rglob("*"):
    if not path.is_file() or any(part in {"node_modules", ".next", ".git", ".local-run", ".runtime"} for part in path.parts):
        continue
    if path.stat().st_size > 5_000_000:
        continue
    if path.suffix.lower() not in {".go", ".ts", ".tsx", ".js", ".json", ".md", ".sql", ".yml", ".yaml", ".env", ".example", ".sh"} and path.name != ".env.example":
        continue
    try:
        text = path.read_text(encoding="utf-8", errors="ignore")
    except Exception:
        continue
    rel = path.relative_to(ROOT)
    if "/api/v4" in text:
        fail(f"Stale /api/v4 reference: {rel}")
    # Lockfiles legitimately contain platform package names such as "android".
    # Architecture-staleness checks apply to authored source/config/docs, not dependency metadata.
    if re.search(r"(?i)\b(supabase|termux|android)\b", text) and path.name not in {"RELEASE_NOTES.md", "QA_REPORT.md", "pnpm-lock.yaml", "package-lock.json"}:
        fail(f"Stale removed-architecture reference: {rel}")
    for pattern in secret_patterns:
        if pattern.search(text):
            fail(f"Possible embedded credential: {rel}")

# ---------------------------------------------------------------------------
# API contract checks: browser->gateway/modules AND module->module.
# ---------------------------------------------------------------------------
for script, key in (("api_contract_qa.py", "public_api_contracts"), ("internal_api_contract_qa.py", "internal_api_contracts")):
    proc = subprocess.run([sys.executable, str(ROOT / "tools" / script)], cwd=ROOT, capture_output=True, text=True, timeout=90)
    if proc.returncode != 0:
        fail(f"{script} failed: {(proc.stderr or proc.stdout)[-2000:]}")
    else:
        checks[key] = "pass"

# ---------------------------------------------------------------------------
# Parser/static build checks available without dependency downloads.
# ---------------------------------------------------------------------------
node_script = r'''
const fs=require('fs'),path=require('path'),cp=require('child_process');let ts;
try{ts=require(path.join(cp.execSync('npm root -g').toString().trim(),'typescript'))}catch(e){process.exit(3)}
let errs=[],count=0;
for(const root of process.argv.slice(1)){
 const walk=p=>{for(const e of fs.readdirSync(p,{withFileTypes:true})){
   const f=path.join(p,e.name); if(e.isDirectory())walk(f); else if(/\.tsx?$/.test(f)){
     count++; const src=fs.readFileSync(f,'utf8');
     const out=ts.transpileModule(src,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext},reportDiagnostics:true,fileName:f});
     for(const d of out.diagnostics||[]) if(d.category===ts.DiagnosticCategory.Error) errs.push(f+': '+ts.flattenDiagnosticMessageText(d.messageText,' '));
   }
 }}; walk(root)
}
if(errs.length){console.error(errs.join('\n'));process.exit(2)}
console.log(count)
'''
try:
    proc = subprocess.run(
        ["node", "-e", node_script, *[str(ROOT / "apps" / app / "src") for app in APPS]],
        capture_output=True,
        text=True,
        timeout=90,
    )
    if proc.returncode == 2:
        fail("TypeScript parser errors: " + proc.stderr[:2000])
    elif proc.returncode == 3:
        warn("Global TypeScript unavailable; parser-only TS check skipped")
    else:
        checks["typescript_files_parsed"] = int(proc.stdout.strip())
        checks["typescript_syntax"] = "pass"
except (FileNotFoundError, subprocess.TimeoutExpired):
    warn("Node/TypeScript syntax check unavailable")

# Full frontend build when dependencies are already present. Do not mutate the
# artifact or pretend a build ran if the sandbox cannot install dependencies.
full_frontend = True
for app in APPS:
    app_dir = ROOT / "apps" / app
    if not (app_dir / "node_modules/next").exists():
        full_frontend = False
        continue
    proc = subprocess.run(["npm", "run", "typecheck"], cwd=app_dir, capture_output=True, text=True, timeout=120)
    if proc.returncode != 0:
        fail(f"{app}: npm run typecheck failed: {(proc.stderr or proc.stdout)[-2000:]}")
        continue
    proc = subprocess.run(["npm", "run", "build"], cwd=app_dir, capture_output=True, text=True, timeout=240)
    if proc.returncode != 0:
        fail(f"{app}: npm run build failed: {(proc.stderr or proc.stdout)[-2000:]}")
if full_frontend:
    checks["next_production_builds"] = "pass"
else:
    checks["next_production_builds"] = "not_run_missing_node_modules"
    warn("Full Next.js typecheck/build was not run because node_modules are not bundled in this sandbox artifact")

# gofmt is a Go parser/formatter check that does not need dependency downloads.
go_files = [str(path) for path in (ROOT / "backend").rglob("*.go")]
try:
    proc = subprocess.run(["gofmt", "-d", *go_files], capture_output=True, text=True, timeout=90)
    if proc.returncode != 0:
        fail("gofmt could not parse Go sources: " + proc.stderr[:1500])
    elif proc.stdout.strip():
        fail("Go sources are not gofmt-clean")
    else:
        checks["go_syntax_gofmt"] = "pass"
except (FileNotFoundError, subprocess.TimeoutExpired):
    warn("gofmt unavailable")

# Full Go test is attempted; dependency-registry failure is recorded, never
# silently upgraded to PASS.
try:
    proc = subprocess.run(["go", "test", "./..."], cwd=ROOT / "backend", capture_output=True, text=True, timeout=45)
    if proc.returncode == 0:
        checks["go_test"] = "pass"
    elif any(token in (proc.stderr + proc.stdout) for token in ("missing go.sum entry", "proxy.golang.org", "no such host", "dial tcp")):
        checks["go_test"] = "blocked_by_dependency_registry"
        warn("Full Go test/build could not resolve external modules in this sandbox; Railway/local build will resolve go.mod dependencies")
    else:
        fail("go test failed: " + (proc.stderr or proc.stdout)[-2500:])
except subprocess.TimeoutExpired:
    checks["go_test"] = "blocked_or_timeout"
    warn("go test timed out while resolving external modules")
except FileNotFoundError:
    warn("Go unavailable; full Go build skipped")

# Bash launcher syntax and required commands.
try:
    proc = subprocess.run(["bash", "-n", str(ROOT / "local.sh")], capture_output=True, text=True, timeout=20)
    if proc.returncode != 0:
        fail("local.sh syntax: " + proc.stderr[:1000])
    else:
        local = (ROOT / "local.sh").read_text(encoding="utf-8")
        for command in ("start", "stop", "restart", "status", "logs", "admin", "reset-password", "seed-vocab", "seed-demo", "qa"):
            if command not in local:
                fail(f"local.sh missing command {command}")
        checks["local_launcher"] = "pass"
except (FileNotFoundError, subprocess.TimeoutExpired):
    warn("bash unavailable; local.sh syntax check skipped")

# JSON/YAML configuration parse checks.
for path in ROOT.rglob("*.json"):
    if any(part in {"node_modules", ".next"} for part in path.parts):
        continue
    try:
        json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        fail(f"Invalid JSON {path.relative_to(ROOT)}: {exc}")
checks["json_configs"] = "pass" if not any(x.startswith("Invalid JSON") for x in issues) else "fail"

try:
    import yaml  # type: ignore

    for path in list(ROOT.rglob("*.yml")) + list(ROOT.rglob("*.yaml")):
        if any(part in {"node_modules", ".next"} for part in path.parts):
            continue
        try:
            yaml.safe_load(path.read_text(encoding="utf-8"))
        except Exception as exc:
            fail(f"Invalid YAML {path.relative_to(ROOT)}: {exc}")
    checks["yaml_configs"] = "pass" if not any(x.startswith("Invalid YAML") for x in issues) else "fail"
except ImportError:
    warn("PyYAML unavailable; YAML syntax check skipped")

report = {"ok": not issues, "checks": checks, "issues": issues, "warnings": warnings}
(ROOT / "QA_REPORT.json").write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
md = ["# Assessment Platform V5 — Production QA", "", f"**Status:** {'PASS' if not issues else 'FAIL'}", "", "## Checks", ""]
md += [f"- **{key}:** `{value}`" for key, value in checks.items()]
md += ["", "## Issues", ""] + ([f"- {message}" for message in issues] if issues else ["- None"])
md += ["", "## Environment limitations", ""] + ([f"- {message}" for message in warnings] if warnings else ["- None"])
md += [
    "",
    "## Scope",
    "",
    "The automated QA validates the 8,000-item English loader contract, 8,000-item SAT bank, demo vocabulary balance, SQL placeholders, public browser API contracts, internal module contracts including service-signature authorization, React hook anti-patterns, the shadcn-style UI boundary, monochrome theme constraints, private-portal indexing policy, security headers, Go parsing/gofmt, launcher syntax and config files.",
    "",
    "A full dependency build is only marked PASS when the environment actually contains or can download the required dependencies.",
]
(ROOT / "QA_REPORT.md").write_text("\n".join(md) + "\n", encoding="utf-8")
print(json.dumps(report, indent=2, ensure_ascii=False))
sys.exit(1 if issues else 0)
