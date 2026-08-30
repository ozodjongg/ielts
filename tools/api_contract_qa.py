#!/usr/bin/env python3
from __future__ import annotations
import json, re, subprocess, sys
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
APPS=['admin-web','center-web','teacher-web','student-web']
PORTAL_ROLE={'admin':'admin','center':'center','teacher':'teacher','student':'student'}

# Backend public /v1 routes per logical service.
routes: dict[str,list[tuple[str,str]]]={}
for p in (ROOT/'backend/modules').glob('*/service.go'):
    service=p.parent.name
    text=p.read_text(encoding='utf-8')
    found=[]
    for m in re.finditer(r'm\.HandleFunc\("(GET|POST|PATCH|PUT|DELETE|HEAD)\s+([^"\s]+)"',text):
        method,path=m.group(1),m.group(2)
        if path.startswith('/v1'):
            found.append((method,path))
    routes[service]=found

# TypeScript AST extractor. It returns frontend api/portalPath calls with inferred method.
node=r'''
const fs=require('fs'),path=require('path'),cp=require('child_process');
let ts; try { ts=require(path.join(cp.execSync('npm root -g').toString().trim(),'typescript')); } catch(e) { console.error(e.message); process.exit(3); }
const roots=process.argv.slice(1); const out=[];
function templ(n){
  if(ts.isStringLiteral(n)||ts.isNoSubstitutionTemplateLiteral(n)) return n.text;
  if(ts.isTemplateExpression(n)){
    let s=n.head.text; for(const sp of n.templateSpans){s+='{param}'+sp.literal.text;} return s;
  }
  return null;
}
function str(n){return (ts.isStringLiteral(n)||ts.isNoSubstitutionTemplateLiteral(n))?n.text:null;}
function walkFile(file){
 const src=fs.readFileSync(file,'utf8'); const sf=ts.createSourceFile(file,src,ts.ScriptTarget.Latest,true,file.endsWith('.tsx')?ts.ScriptKind.TSX:ts.ScriptKind.TS);
 function visit(n){
  if(ts.isCallExpression(n) && ts.isIdentifier(n.expression) && (n.expression.text==='api'||n.expression.text==='apiBlob')){
    const fn=n.expression.text; const first=n.arguments[0];
    if(first && ts.isCallExpression(first) && ts.isIdentifier(first.expression) && first.expression.text==='portalPath'){
      const portal=str(first.arguments[0]), service=str(first.arguments[1]); const suffix=first.arguments[2]?templ(first.arguments[2]):'';
      let method='GET';
      if(fn==='api' && n.arguments[2]){
        const init=n.arguments[2];
        if(ts.isCallExpression(init)&&ts.isIdentifier(init.expression)&&init.expression.text==='json') method=str(init.arguments[0])||method;
        else if(ts.isObjectLiteralExpression(init)) for(const p of init.properties){ if(ts.isPropertyAssignment(p)&&p.name&&p.name.getText(sf).replace(/["']/g,'')==='method') method=str(p.initializer)||method; }
      }
      out.push({file:path.relative(process.cwd(),file).replace(/\\/g,'/'),line:sf.getLineAndCharacterOfPosition(n.getStart(sf)).line+1,portal,service,suffix,method});
    }
  }
  ts.forEachChild(n,visit);
 }
 visit(sf);
}
function walk(root){for(const e of fs.readdirSync(root,{withFileTypes:true})){const f=path.join(root,e.name);if(e.isDirectory())walk(f);else if(/\.tsx?$/.test(f))walkFile(f)}}
for(const r of roots)walk(r); console.log(JSON.stringify(out));
'''
try:
    proc=subprocess.run(['node','-e',node,*[str(ROOT/'apps'/a/'src') for a in APPS]],cwd=ROOT,capture_output=True,text=True,timeout=90)
except Exception as e:
    print(json.dumps({'ok':False,'issues':[f'AST extractor failed: {e}']})); sys.exit(1)
if proc.returncode:
    print(json.dumps({'ok':False,'issues':[f'AST extractor failed: {proc.stderr[:1000]}']})); sys.exit(1)
calls=json.loads(proc.stdout)

def norm_front(suffix:str)->str:
    suffix=(suffix or '')
    suffix=suffix.split('?',1)[0]
    suffix=re.sub(r'\{param\}', '{x}', suffix)
    if suffix and not suffix.startswith('/'): suffix='/'+suffix
    return '/v1'+suffix

def route_re(pat:str):
    # Go ServeMux placeholders: {id}. Frontend normalized placeholders: {x}.
    esc=''
    pos=0
    for m in re.finditer(r'\{[^}]+\}',pat):
        esc+=re.escape(pat[pos:m.start()])+r'[^/]+'
        pos=m.end()
    esc+=re.escape(pat[pos:])
    return re.compile('^'+esc+'$')

issues=[]; matched=[]; dynamic_matched=[]
for c in calls:
    portal,service,method=c.get('portal'),c.get('service'),c.get('method')
    # AuthProvider uses portalPath(portal, "identity", "/me") where portal is
    # a runtime union of admin|center|teacher|student. Verify that common module route
    # independently instead of dropping it from the contract count.
    if not portal and service:
        down=norm_front(c.get('suffix') or '')
        candidates=routes.get(service,[])
        ok=any(method==bm and route_re(bp).match(down) for bm,bp in candidates)
        if not ok:
            issues.append(f"{c['file']}:{c['line']} dynamic portal {method} {service}{c.get('suffix') or ''} has no backend route")
        else:
            dynamic_matched.append(c)
        continue
    if not portal or not service:
        issues.append(f"{c['file']}:{c['line']} frontend API call could not be resolved statically")
        continue
    down=norm_front(c.get('suffix') or '')
    candidates=routes.get(service,[])
    ok=any(method==bm and route_re(bp).match(down) for bm,bp in candidates)
    if not ok:
        issues.append(f"{c['file']}:{c['line']} {method} /api/{portal}/{service}{c.get('suffix') or ''} has no backend {service} route (maps to {down})")
    else:
        matched.append(c)

# Validate gateway service exposure against frontend calls.
gateway=(ROOT/'backend/modules/gateway/service.go').read_text(encoding='utf-8')
allowed={
 'admin': set('identity tenant analytics vocabulary points assessment sat'.split()),
 'center': set('identity tenant assessment listening review sat analytics points vocabulary'.split()),
 'teacher': set('identity tenant vocabulary'.split()),
 'student': set('identity assessment vocabulary listening review sat points analytics tenant'.split()),
}
for c in matched:
    if c['service'] not in allowed.get(c['portal'],set()):
        issues.append(f"{c['file']}:{c['line']} service {c['service']} is not exposed to {c['portal']} portal")
    if c['portal']=='student' and c['service']=='tenant' and norm_front(c.get('suffix') or '')!='/v1/services':
        issues.append(f"{c['file']}:{c['line']} student tenant API exceeds gateway read-only /services policy")

# Auth/security helpers intentionally use a runtime portal union across all four portals.
_dynamic_identity = {'/v1/me','/v1/mfa/status','/v1/mfa/setup','/v1/mfa/verify','/v1/mfa/disable'}
for c in dynamic_matched:
    if c.get('service')!='identity' or norm_front(c.get('suffix') or '') not in _dynamic_identity:
        issues.append(f"{c['file']}:{c['line']} unexpected dynamic portal API call")

# AuthProvider has 4 direct gateway auth calls per app. Verify the templates and
# the gateway's single constrained route.
auth_calls=[]
gateway_auth_route=bool(re.search(r'm\.HandleFunc\("POST /auth/\{portal\}/\{action\}"', gateway))
if not gateway_auth_route:
    issues.append('gateway: missing POST /auth/{portal}/{action}')
for app in APPS:
    p=ROOT/'apps'/app/'src/components/auth-provider.tsx'
    text=p.read_text(encoding='utf-8')
    for action in ('login','refresh','logout','mfa-verify'):
        pattern=f'/auth/${{portal}}/{action}'
        count=text.count(pattern)
        if count != 1:
            issues.append(f'{app}: expected exactly one {pattern} call, found {count}')
        else:
            auth_calls.append({'file':str(p.relative_to(ROOT)).replace('\\','/'),'portal':'dynamic','action':action,'method':'POST'})
    if 'typePortal="admin"|"center"|"teacher"|"student"' not in text.replace(' ', '').replace('\n',''):
        issues.append(f'{app}: AuthProvider portal type must be admin|center|teacher|student')

# Verify backend gateway enforces only those 4 auth actions and role mapping.
for token in ('action != "login"', 'action != "refresh"', 'action != "logout"', 'action != "mfa-verify"'):
    if token not in gateway:
        issues.append(f'gateway: auth action guard missing {token}')
for portal,role in PORTAL_ROLE.items():
    if f'case "{portal}":' not in gateway or f'return "{role}", true' not in gateway:
        issues.append(f'gateway: missing portal role mapping {portal}->{role}')

# Frontend defensive contract: no dynamic undefined/null path segments should be allowed by API client.
for app in APPS:
    api=(ROOT/'apps'/app/'src/lib/api.ts').read_text(encoding='utf-8')
    if 'undefined|null' not in api or 'Refusing invalid API path' not in api:
        issues.append(f'{app}: API client does not reject undefined/null URL segments')
    if 'NEXT_PUBLIC_API_URL is required in production' not in api:
        issues.append(f'{app}: production API URL is not required explicitly')

report={
    'ok':not issues,
    'frontend_module_calls':len(calls),
    'frontend_auth_calls':len(auth_calls),
    'matched_module_calls':len(matched)+len(dynamic_matched),
    'backend_public_routes':sum(len(v) for v in routes.values()),
    'issues':issues,
    'calls':calls,
    'auth_calls':auth_calls,
}
(ROOT/'API_CONTRACT_REPORT.json').write_text(json.dumps(report,indent=2,ensure_ascii=False)+'\n',encoding='utf-8')
md=[
    '# API Contract QA', '',
    f"**Status:** {'PASS' if not issues else 'FAIL'}", '',
    f"- Frontend module calls checked: {len(calls)}",
    f"- Frontend auth calls checked: {len(auth_calls)}",
    f"- Module calls matched: {len(matched)+len(dynamic_matched)}",
    f"- Backend public module routes: {sum(len(v) for v in routes.values())}", '',
    '## Issues', '',
]
md += [f'- {x}' for x in issues] if issues else ['- None']
(ROOT/'API_CONTRACT_REPORT.md').write_text('\n'.join(md)+'\n',encoding='utf-8')
print(json.dumps(report,indent=2,ensure_ascii=False))
sys.exit(0 if not issues else 1)
