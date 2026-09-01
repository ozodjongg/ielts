#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
EXPECTED_IDENTITY_001 = "977337be68614988ce45fe05542a5e88a192fea106d85e8eab0b616bdacb8282"

failures: list[str] = []
passes: list[str] = []


def ok(label: str, condition: bool, detail: str = "") -> None:
    if condition:
        passes.append(label)
        print(f"[PASS] {label}{': ' + detail if detail else ''}")
    else:
        failures.append(label)
        print(f"[FAIL] {label}{': ' + detail if detail else ''}")


def text(path: str) -> str:
    p = ROOT / path
    return p.read_text(encoding="utf-8", errors="replace") if p.exists() else ""


def runtime_files(base: pathlib.Path):
    for p in base.rglob("*"):
        if not p.is_file():
            continue
        if any(part in {"node_modules", ".next", ".git"} for part in p.parts):
            continue
        if p.suffix.lower() not in {".go", ".ts", ".tsx", ".js", ".jsx"}:
            continue
        yield p

# 1) Student has no MFA/TOTP/security runtime references.
student_root = ROOT / "apps/student-web/src"
student_blob = "\n".join(p.read_text(encoding="utf-8", errors="replace") for p in runtime_files(student_root))
student_mfa_refs = re.findall(r"(?i)\b(?:mfa|totp|authenticator)\b|/security", student_blob)
ok("student MFA removed", len(student_mfa_refs) == 0, f"refs={len(student_mfa_refs)}")

# 2) MFA roles are privileged only and login checks role before MFA.
totp = text("backend/modules/identity/totp.go")
login = text("backend/modules/identity/local_auth.go")
ok("MFA role scope admin/center/teacher", 'role == "admin" || role == "center" || role == "teacher"' in totp)
ok("login MFA gated by privileged role", "if validMFARole(p.Role)" in login)

# 3) AAL2 sensitive mutation policy includes all privileged roles and no student switch.
gateway = text("backend/modules/gateway/service.go")
main = text("backend/cmd/backend/main.go")
ok("AAL2 gateway admin", 'role == "admin" && s.RequireAdminAAL2' in gateway)
ok("AAL2 gateway center", 'role == "center" && s.RequireCenterAAL2' in gateway)
ok("AAL2 gateway teacher", 'role == "teacher" && s.RequireTeacherAAL2' in gateway)
ok("no student AAL2 config", "RequireStudentAAL2" not in gateway + main)

# 4) QR setup exists for admin/center/teacher and not student.
for app in ("admin-web", "center-web", "teacher-web"):
    security = text(f"apps/{app}/src/app/(portal)/security/page.tsx")
    ok(f"{app} QR setup", bool(re.search(r"(?i)qr|QRCode|qr-code", security)))
ok("student Security page absent", not (ROOT / "apps/student-web/src/app/(portal)/security/page.tsx").exists())

# 5) vocabulary_test absent from runtime source. Historical/new migration mentions are allowed.
runtime_blob = []
for base in (ROOT / "backend", ROOT / "apps"):
    for p in runtime_files(base):
        runtime_blob.append(p.read_text(encoding="utf-8", errors="replace"))
refs = re.findall(r"\bvocabulary_test\b", "\n".join(runtime_blob))
ok("vocabulary_test removed from runtime", len(refs) == 0, f"refs={len(refs)}")
ok("vocabulary_test cleanup migration", "DELETE FROM service_catalog WHERE code='vocabulary_test'" in text("backend/migrations/tenant/004_group_teachers_and_service_cleanup.sql"))

# 6) Real many-to-many teacher/group model + server authorization.
tenant_mig = text("backend/migrations/tenant/004_group_teachers_and_service_cleanup.sql")
tenant = text("backend/modules/tenant/service.go")
ok("group_teachers table", "CREATE TABLE IF NOT EXISTS group_teachers" in tenant_mig)
ok("teacher owns group authorization", "teacherOwnsGroup" in tenant and "group_teachers" in tenant)
ok("teacher owns student authorization", "teacherOwnsStudent" in tenant)
ok("teacher cannot target all", "teachers can assign services only to their groups or students in those groups" in tenant)

# 7) Teacher assignment support in English/SAT/Listening.
for module in ("assessment", "sat", "listening"):
    svc = text(f"backend/modules/{module}/service.go")
    ok(f"{module} teacher assignment", 'a.Role != "center" && a.Role != "teacher"' in svc)
    ok(f"{module} teacher all-target blocked", "teachers can assign only to their own groups or students in those groups" in svc)

# 8) Listening center UI is form-based, not raw JSON input.
listening = text("apps/center-web/src/app/(portal)/listening/page.tsx")
ok("Listening normal question builder", all(x in listening for x in ("Add question", "Add option", "answerIndex", "basePoints")))
ok("Listening explains no JSON", "JSON yozish talab qilinmaydi" in listening)

# 9) Unified services routes + legacy redirects.
for app in ("center-web", "student-web", "teacher-web"):
    ok(f"{app} Services page", (ROOT / f"apps/{app}/src/app/(portal)/services/page.tsx").exists())
for app, legacy in (("center-web", "assessments"), ("center-web", "sat"), ("student-web", "english"), ("student-web", "sat")):
    p = ROOT / f"apps/{app}/src/app/(portal)/{legacy}/page.tsx"
    ok(f"{app} legacy /{legacy} redirect", p.exists() and "/services" in p.read_text(encoding="utf-8", errors="replace"))

# 10) Local runner knows all apps + privileged MFA defaults.
local = text("local.sh")
for app in ("admin-web", "center-web", "student-web", "teacher-web"):
    ok(f"local.sh {app}", f"apps/{app}" in local)
for role in ("ADMIN", "CENTER", "TEACHER"):
    ok(f"local.sh REQUIRE_{role}_AAL2", f"REQUIRE_{role}_AAL2" in local)
ok("local.sh no student AAL2", "REQUIRE_STUDENT_AAL2" not in local)

# 11) Preserve current Railway migration lineage.
identity_001 = ROOT / "backend/migrations/identity/001_init.sql"
actual = hashlib.sha256(identity_001.read_bytes()).hexdigest() if identity_001.exists() else "missing"
ok("identity/001 Railway checksum lineage", actual == EXPECTED_IDENTITY_001, actual)

# 12) Regression fix: manual Mock responses retain service_code.
assessment = text("backend/modules/assessment/service.go")
english_ui = text("apps/student-web/src/components/services/english-services.tsx")
ok("Mock manual response carries service_code", '"mode": "manual", "service_code": x.ServiceCode' in assessment)
ok("student preserves service_code", "response.service_code ?? previous?.service_code" in english_ui)
ok("Finish gated by submitted required prompts", "allRequiredSubmitted" in english_ui and "Finish assessment" in english_ui)

# 13) Current prompt.ai: pre-registration placement before student account creation.
placement_mig = text("backend/migrations/assessment/004_pre_registration_placement.sql")
placement_backend = text("backend/modules/assessment/service.go")
placement_ui = text("apps/center-web/src/app/(portal)/placement/page.tsx")
students_ui = text("apps/center-web/src/app/(portal)/students/page.tsx")
ok("pre-registration placement table", "CREATE TABLE IF NOT EXISTS pre_registration_placements" in placement_mig)
ok("center placement API routes", all(route in placement_backend for route in (
    'GET /v1/pre-registration/placements',
    'POST /v1/pre-registration/placements',
    'POST /v1/pre-registration/placements/{id}/finish',
    'POST /v1/pre-registration/placements/{id}/registered',
    'GET /v1/pre-registration/placement-paper',
)))
ok("center placement page", "pre-registration/placements" in placement_ui and "current_level: result.level" in placement_ui)
ok("paper placement download UI", "placement-paper" in placement_ui and "Word testni yuklab olish" in placement_ui)
ok("students creation starts from placement", 'router.push("/placement")' in students_ui and "Initial level" not in students_ui)

# 14) Current prompt.ai: Reviews is teacher-friendly and contains no raw rubric JSON editor.
reviews_ui = text("apps/center-web/src/app/(portal)/reviews/page.tsx")
ok("Reviews has no raw rubric JSON", "Rubric JSON" not in reviews_ui and "JSON.parse" not in reviews_ui)
ok("Reviews simple criterion inputs", all(x in reviews_ui for x in ("Task response", "Coherence & cohesion", "Pronunciation", "0–25")))

# 15) Current prompt.ai: printable Word artifact and investor/center presentation are installed.
paper_docx = ROOT / "data/placement/placement-test-paper.docx"
paper_manifest = ROOT / "data/placement/paper-v1.json"
presentation = ROOT / "presentation/presentation.pptx"
ok("printable placement Word file", paper_docx.exists() and paper_docx.stat().st_size > 10_000)
ok("paper placement manifest", paper_manifest.exists() and "question_ids" in text("data/placement/paper-v1.json"))
ok("investor/center presentation", presentation.exists() and presentation.stat().st_size > 50_000)

# 16) Follow-up prompt: digital placement uses one-time QR invitation on candidate phone.
placement_invite_mig = text("backend/migrations/assessment/005_placement_invitation_tokens.sql")
gateway = text("backend/modules/gateway/service.go")
center_placement = text("apps/center-web/src/app/(portal)/placement/page.tsx")
invite_page = text("apps/center-web/src/app/placement/invite/page.tsx")
candidate_page = text("apps/center-web/src/app/placement/test/page.tsx")
center_pkg = text("apps/center-web/package.json")
ok("placement invitation token hash migration", "invitation_token_hash" in placement_invite_mig and "candidate_session_hash" in placement_invite_mig)
ok("public placement gateway is narrow", all(x in gateway for x in ("invitations/claim", "session/answer", "session/finish", "PublicPlacementLimiter", "X-Placement-Session")))
ok("raw invitation token not persisted", "invitation_token_hash=NULL" in placement_backend and "placementTokenHash" in placement_backend)
ok("center digital QR invitation UI", "QRCodeSVG" in center_placement and "/placement/invite#token=" in center_placement and "Invitation yaratish" in center_placement)
ok("candidate invitation claim page", "/public/placement/invitations/claim" in invite_page and "window.location.hash" in invite_page and "replaceState" in invite_page)
ok("candidate phone test page", all(x in candidate_page for x in ("/public/placement/session", "session/answer", "session/finish", "X-Placement-Session")))
ok("QR dependency locked", '"qrcode.react": "^4.2.0"' in center_pkg and "qrcode.react@4.2.0" in text("apps/center-web/pnpm-lock.yaml"))

print(f"\nSummary: {len(passes)} passed, {len(failures)} failed")
if failures:
    print("Failures:")
    for item in failures:
        print(f" - {item}")
    sys.exit(1)
