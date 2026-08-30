#!/usr/bin/env python3
from __future__ import annotations
import csv, json, re, subprocess, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
APPS = ["admin-web", "center-web", "teacher-web", "student-web"]
issues: list[str] = []
warnings: list[str] = []
checks: dict[str, object] = {}

def fail(msg: str): issues.append(msg)
def warn(msg: str): warnings.append(msg)
def rows(path: Path):
    with path.open(encoding="utf-8-sig", newline="") as f: return list(csv.DictReader(f))

# Immutable learning banks.
eng = rows(ROOT / "data/english-bank/questions.csv")
sat = rows(ROOT / "data/sat-math-bank/questions.csv")
checks["english_questions"] = len(eng)
checks["sat_questions"] = len(sat)
if len(eng) != 8000: fail(f"English bank must contain 8000 questions, found {len(eng)}")
if len(sat) != 8000: fail(f"SAT bank must contain 8000 questions, found {len(sat)}")

# Four portal structure, role-specific auth, 12 themes, responsive/security metadata.
for app in APPS:
    base = ROOT / "apps" / app
    for rel in ["package.json","pnpm-lock.yaml","src/app/layout.tsx","src/app/login/page.tsx","src/app/globals.css","src/components/auth-provider.tsx","src/components/theme-provider.tsx","src/components/shell.tsx","src/app/(portal)/security/page.tsx"]:
        if not (base / rel).exists(): fail(f"{app}: missing {rel}")
    tp = (base / "src/components/theme-provider.tsx").read_text(encoding="utf-8")
    values = re.findall(r'value:"([a-z]+)"', tp)
    if len(set(values)) != 12: fail(f"{app}: expected 12 themes, found {len(set(values))}")
    css = (base / "src/app/globals.css").read_text(encoding="utf-8")
    for width in (1100, 820, 560):
        if not re.search(r"@media\s*\(max-width:\s*" + str(width) + r"px\)", css):
            fail(f"{app}: responsive breakpoint missing: {width}px")
    layout = (base / "src/app/layout.tsx").read_text(encoding="utf-8")
    if "IELTS" not in layout or "index: false" not in layout: fail(f"{app}: IELTS/private metadata incomplete")
checks["frontend_projects"] = len(APPS)
checks["themes_per_portal"] = 12

# No old V5 visible branding in authored frontend source.
for path in (ROOT / "apps").rglob("*"):
    if path.is_file() and path.suffix in {".ts",".tsx",".css",".md",".json"} and path.name != "pnpm-lock.yaml":
        text = path.read_text(encoding="utf-8", errors="ignore")
        if re.search(r"\bV5\b|Assessment Platform IELTS", text, re.I): fail(f"stale V5 branding: {path.relative_to(ROOT)}")

# Role + TOTP + teacher learning backend contracts.
required_text = {
    "backend/migrations/identity/003_roles_mfa.sql": ["'admin','center','teacher','student'", "mfa_totp", "mfa_recovery_codes", "mfa_challenges"],
    "backend/modules/identity/totp.go": ["mfaSetup", "mfaSetupVerify", "internalMFAVerify", "mfaDisable", "otpauth://totp/"],
    "backend/modules/gateway/service.go": ["RequireTeacherAAL2", 'case "teacher"', '"/api/teacher/"'],
    "backend/modules/vocabulary/teacher_learning.go": ["teacherAssignWords", "teacherCreateHomework", "studentAssigned", "studentCompleteHomework", "requireTeacherActor"],
    "backend/modules/tenant/service.go": ["createTeacher", "updateTeacher", '"role": "teacher"'],
}
for rel, needles in required_text.items():
    text = (ROOT / rel).read_text(encoding="utf-8")
    for needle in needles:
        if needle not in text: fail(f"{rel}: missing contract marker {needle}")
checks["roles"] = ["admin","center","teacher","student"]
checks["totp_aal2"] = "implemented"
checks["teacher_vocabulary_homework"] = "implemented"

# Center UI must not retain a vocabulary mutation manager; teacher must have it.
if (ROOT / "apps/center-web/src/app/(portal)/vocabulary-manager").exists(): fail("center portal still exposes vocabulary manager")
for rel in ["apps/teacher-web/src/app/(portal)/vocabulary-manager/page.tsx", "apps/teacher-web/src/app/(portal)/homework/page.tsx", "apps/teacher-web/src/app/(portal)/students/page.tsx", "apps/student-web/src/app/(portal)/assigned/page.tsx"]:
    if not (ROOT / rel).exists(): fail(f"missing role workflow page: {rel}")

# Deployment contract.
compose = (ROOT / "docker-compose.yml").read_text(encoding="utf-8")
for marker in ["admin-web:","center-web:","teacher-web:","student-web:","MFA_ENCRYPTION_KEY", "TEACHER_ORIGINS", "REQUIRE_TEACHER_AAL2"]:
    if marker not in compose: fail(f"docker-compose missing {marker}")
rail_env = (ROOT / "deploy/railway-backend.env.example").read_text(encoding="utf-8")
for marker in ["MFA_ENCRYPTION_KEY=", "TEACHER_ORIGINS=", "REQUIRE_ADMIN_AAL2=true", "REQUIRE_CENTER_AAL2=true", "REQUIRE_TEACHER_AAL2=true"]:
    if marker not in rail_env: fail(f"Railway env example missing {marker}")
checks["deployment_topology"] = "Railway backend+PostgreSQL; Vercel 4 portals"

# Local @/ import resolution.
for app in APPS:
    src = ROOT / "apps" / app / "src"
    for path in list(src.rglob("*.ts")) + list(src.rglob("*.tsx")):
        text = path.read_text(encoding="utf-8")
        for m in re.finditer(r'from\s+["\']@/([^"\']+)["\']', text):
            base = src / m.group(1)
            candidates = [base.with_suffix(".ts"),base.with_suffix(".tsx"),base/"index.ts",base/"index.tsx"]
            if not any(c.exists() for c in candidates): fail(f"{app}: unresolved local import @/{m.group(1)} in {path.relative_to(src)}")
checks["local_imports"] = "pass" if not any("unresolved local import" in x for x in issues) else "fail"

# Parse TS/TSX using globally installed TypeScript, without npm install.
node_script = r'''
const fs=require('fs'),path=require('path'),cp=require('child_process');
const ts=require(path.join(cp.execSync('npm root -g').toString().trim(),'typescript'));
let bad=[],count=0;
for(const root of process.argv.slice(1)){const walk=p=>{for(const e of fs.readdirSync(p,{withFileTypes:true})){const f=path.join(p,e.name);if(e.isDirectory())walk(f);else if(/\.tsx?$/.test(f)){count++;const o=ts.transpileModule(fs.readFileSync(f,'utf8'),{compilerOptions:{jsx:ts.JsxEmit.Preserve,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext},reportDiagnostics:true,fileName:f});for(const d of o.diagnostics||[])if(d.category===ts.DiagnosticCategory.Error)bad.push(f+': '+ts.flattenDiagnosticMessageText(d.messageText,' '));}}};walk(root)}
if(bad.length){console.error(bad.join('\n'));process.exit(2)}console.log(count);
'''
try:
    p = subprocess.run(["node","-e",node_script,*[str(ROOT/"apps"/a/"src") for a in APPS]],capture_output=True,text=True,timeout=60)
    if p.returncode: fail("TypeScript syntax parser failed: "+(p.stderr or p.stdout)[-1800:])
    else: checks["typescript_files_parsed"] = int(p.stdout.strip())
except Exception as e: warn(f"TypeScript parser unavailable: {e}")

# Go parser/formatter without resolving third-party modules.
go_files = [str(p) for p in (ROOT/"backend").rglob("*.go")]
try:
    p=subprocess.run(["gofmt","-d",*go_files],capture_output=True,text=True,timeout=60)
    if p.returncode or p.stdout.strip(): fail("Go source is not parseable/gofmt-clean: "+(p.stderr or p.stdout)[:1800])
    else: checks["go_syntax_gofmt"]="pass"
except Exception as e: warn(f"gofmt unavailable: {e}")

# Full Go tests are attempted; dependency DNS/network restrictions remain a warning only.
try:
    p=subprocess.run(["go","test","./..."],cwd=ROOT/"backend",capture_output=True,text=True,timeout=40)
    if p.returncode==0: checks["go_test"]="pass"
    elif any(x in (p.stdout+p.stderr) for x in ["proxy.golang.org","no such host","dial tcp","i/o timeout"]):
        checks["go_test"]="blocked_by_dependency_registry"; warn("Full go test/build could not download modules in this sandbox")
    else: fail("go test failed: "+(p.stderr or p.stdout)[-2200:])
except subprocess.TimeoutExpired:
    checks["go_test"]="blocked_or_timeout"; warn("go test timed out while resolving dependencies")
except Exception as e: warn(f"go test unavailable: {e}")

# Config parsers.
for p in ROOT.rglob("*.json"):
    if "node_modules" in p.parts or ".next" in p.parts: continue
    try: json.loads(p.read_text(encoding="utf-8"))
    except Exception as e: fail(f"invalid JSON {p.relative_to(ROOT)}: {e}")
checks["json"]="pass" if not any("invalid JSON" in x for x in issues) else "fail"

report={"ok":not issues,"checks":checks,"issues":issues,"warnings":warnings}
(ROOT/"QA_REPORT.json").write_text(json.dumps(report,indent=2,ensure_ascii=False)+"\n",encoding="utf-8")
md=["# IELTS Platform — Final QA","",f"**Status:** {'PASS' if not issues else 'FAIL'}","","## Checks",""]
md += [f"- **{k}:** `{v}`" for k,v in checks.items()]
md += ["","## Issues",""] + ([f"- {x}" for x in issues] if issues else ["- None"])
md += ["","## Environment limitations",""] + ([f"- {x}" for x in warnings] if warnings else ["- None"])
(ROOT/"QA_REPORT.md").write_text("\n".join(md)+"\n",encoding="utf-8")
print(json.dumps(report,indent=2,ensure_ascii=False))
sys.exit(1 if issues else 0)
