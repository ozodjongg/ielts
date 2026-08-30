#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MODULES = ROOT / "backend" / "modules"


def route_regex(path: str) -> re.Pattern[str]:
    out = ""
    pos = 0
    for m in re.finditer(r"\{[^}]+\}", path):
        out += re.escape(path[pos:m.start()]) + r"[^/]+"
        pos = m.end()
    out += re.escape(path[pos:])
    return re.compile(r"^" + out + r"$")


def strip_query(path: str) -> str:
    return path.split("?", 1)[0]


def split_top_level(expr: str, delimiter: str = ",") -> list[str]:
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
        elif ch == delimiter and depth == 0:
            parts.append(expr[start:i].strip())
            start = i + 1
    parts.append(expr[start:].strip())
    return [p for p in parts if p]


def extract_balanced(source: str, open_pos: int, opener: str, closer: str) -> tuple[str, int] | None:
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


def string_literal(value: str) -> str | None:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] == '"':
        # Internal route fragments are ASCII. The lightweight unescape below is
        # enough for Go quoted strings used by this project.
        return value[1:-1].replace(r"\/", "/").replace(r'\"', '"').replace(r"\\", "\\")
    return None


def split_top_level_plus(expr: str) -> list[str]:
    return split_top_level(expr, "+")


def normalize_path_expr(expr: str) -> str | None:
    pieces = split_top_level_plus(expr)
    if not pieces:
        return None
    out = ""
    last_dynamic = False
    for piece in pieces:
        lit = string_literal(piece)
        if lit is not None:
            out += lit
            last_dynamic = False
        else:
            if not last_dynamic:
                out += "{x}"
            last_dynamic = True
    return strip_query(out)


def function_body(module: str, name: str) -> str | None:
    for go_file in (MODULES / module).glob("*.go"):
        source = go_file.read_text(encoding="utf-8")
        m = re.search(rf"func\s+\(s\s+\*Service\)\s+{re.escape(name)}\s*\([^)]*\)[^{{]*\{{", source)
        if not m:
            continue
        open_brace = source.find("{", m.start())
        balanced = extract_balanced(source, open_brace, "{", "}")
        if balanced:
            return balanced[0]
    return None


def quoted_args(fragment: str) -> set[str]:
    return set(re.findall(r'"([a-z_]+)"', fragment))


def allowed_callers(module: str, handler: str, cache: dict[tuple[str, str], set[str] | None]) -> set[str] | None:
    key = (module, handler)
    if key in cache:
        return cache[key]
    body = function_body(module, handler)
    if body is None:
        cache[key] = None
        return None

    # Direct serviceAuth helper calls.
    m = re.search(r"s\.serviceAuth\(r,\s*((?:\"[^\"]+\"\s*,?\s*)+)\)", body)
    if m:
        cache[key] = quoted_args(m.group(1))
        return cache[key]

    # Direct authz.VerifyService calls.
    m = re.search(r"authz\.VerifyService\(r,\s*s\.InternalSecret,\s*((?:\"[^\"]+\"\s*,?\s*)+)\)", body)
    if m:
        cache[key] = quoted_args(m.group(1))
        return cache[key]

    # points quote/record use a shared service(r) helper.
    if "s.service(r)" in body:
        helper = function_body(module, "service")
        if helper:
            m = re.search(r"authz\.VerifyService\(r,\s*s\.InternalSecret,\s*((?:\"[^\"]+\"\s*,?\s*)+)\)", helper)
            if m:
                cache[key] = quoted_args(m.group(1))
                return cache[key]

    cache[key] = None
    return None


# Collect internal routes plus their handlers.
routes: dict[str, list[dict[str, str]]] = {}
for service_file in MODULES.glob("*/service.go"):
    module = service_file.parent.name
    text = service_file.read_text(encoding="utf-8")
    found: list[dict[str, str]] = []
    for m in re.finditer(
        r'm\.HandleFunc\("(GET|POST|PATCH|PUT|DELETE|HEAD)\s+([^"\s]+)"\s*,\s*webx\.Handle\(s\.([A-Za-z0-9_]+)\)\)',
        text,
    ):
        method, path, handler = m.group(1), m.group(2), m.group(3)
        if path.startswith("/internal/"):
            found.append({"method": method, "path": path, "handler": handler})
    routes[module] = found

receiver_module = {
    "Identity": "identity",
    "Tenant": "tenant",
    "Assessment": "assessment",
    "Vocabulary": "vocabulary",
    "Listening": "listening",
    "Review": "review",
    "SAT": "sat",
    "Points": "points",
    "Analytics": "analytics",
}

calls: list[dict[str, object]] = []
issues: list[str] = []
auth_cache: dict[tuple[str, str], set[str] | None] = {}


def check_call(*, file: str, line: int, caller: str, target: str, method: str, path: str, source: str = "clientx") -> None:
    call = {
        "file": file,
        "line": line,
        "caller": caller,
        "target": target,
        "method": method,
        "path": path,
        "source": source,
    }
    calls.append(call)
    candidates = routes.get(target, [])
    matched = next(
        (
            route
            for route in candidates
            if method == route["method"] and route_regex(route["path"]).match(path)
        ),
        None,
    )
    if not matched:
        issues.append(f"{file}:{line}: {caller} -> {target} {method} {path} has no matching internal route")
        return
    allowed = allowed_callers(target, matched["handler"], auth_cache)
    if allowed is None:
        issues.append(f"{target} {method} {matched['path']}: internal handler {matched['handler']} has no statically verified service-auth guard")
    elif caller not in allowed:
        issues.append(
            f"{file}:{line}: {caller} is not accepted by {target} {method} {matched['path']} (allowed: {sorted(allowed)})"
        )


for go_file in MODULES.glob("*/*.go"):
    source = go_file.read_text(encoding="utf-8")
    for m in re.finditer(r"s\.(Identity|Tenant|Assessment|Vocabulary|Listening|Review|SAT|Points|Analytics)\.Do\(", source):
        receiver = m.group(1)
        call = extract_balanced(source, m.end() - 1, "(", ")")
        if call is None:
            issues.append(f"{go_file.relative_to(ROOT)}: could not parse {receiver}.Do call")
            continue
        body, _end = call
        args = split_top_level(body)
        if len(args) < 3:
            issues.append(f"{go_file.relative_to(ROOT)}: malformed {receiver}.Do call")
            continue
        method = string_literal(args[1])
        path = normalize_path_expr(args[2])
        line = source.count("\n", 0, m.start()) + 1
        caller = go_file.parent.name
        target = receiver_module[receiver]
        if method is None or path is None:
            issues.append(f"{go_file.relative_to(ROOT)}:{line}: dynamic internal method/path could not be verified")
            continue
        check_call(
            file=str(go_file.relative_to(ROOT)).replace("\\", "/"),
            line=line,
            caller=caller,
            target=target,
            method=method,
            path=path,
        )

# Gateway dispatches auth directly through the identity handler rather than
# clientx, so include these four contracts explicitly.
gateway_file = MODULES / "gateway" / "service.go"
gateway_text = gateway_file.read_text(encoding="utf-8")
for action in ("login", "refresh", "logout", "mfa-verify"):
    check_call(
        file=str(gateway_file.relative_to(ROOT)).replace("\\", "/"),
        line=gateway_text.find('path := "/internal/auth/"') and gateway_text[: gateway_text.find('path := "/internal/auth/"')].count("\n") + 1,
        caller="gateway",
        target="identity",
        method="POST",
        path=f"/internal/auth/{action}",
        source="direct_handler",
    )

report = {
    "ok": not issues,
    "internal_calls": len(calls),
    "internal_routes": sum(len(v) for v in routes.values()),
    "service_auth_guards_verified": len(calls) - sum(1 for i in issues if "service-auth guard" in i),
    "issues": issues,
    "calls": calls,
}
(ROOT / "INTERNAL_API_CONTRACT_REPORT.json").write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
md = [
    "# Internal API Contract QA",
    "",
    f"**Status:** {'PASS' if not issues else 'FAIL'}",
    "",
    f"- Internal calls checked: {len(calls)}",
    f"- Internal routes discovered: {sum(len(v) for v in routes.values())}",
    "- Route existence and service-to-service signature authorization are both checked.",
    "",
    "## Issues",
    "",
]
md += [f"- {x}" for x in issues] if issues else ["- None"]
(ROOT / "INTERNAL_API_CONTRACT_REPORT.md").write_text("\n".join(md) + "\n", encoding="utf-8")
print(json.dumps(report, indent=2, ensure_ascii=False))
sys.exit(0 if not issues else 1)
