# Release Notes — Production V8 Final

## Production hardening

- Combined backend, Admin, Center and Student into one deployable monorepo.
- Railway root Dockerfile now builds backend, migration, admin-bootstrap and vocabulary-import binaries.
- Docker startup now runs migrations before the backend and can auto-import clean vocabulary.
- Added migration history with SHA-256 drift detection and PostgreSQL advisory locking.
- Added vocabulary dataset version/checksum registry and concurrent-import claim protection.
- Removed the noisy OPUS sentence corpus from the production package.
- Added clean 180-lexeme bootstrap vocabulary and synonym data.
- Added Stage 1 WordNet/CEFR and Stage 2 PanLex collector scripts.
- Required signing secrets are validated at startup.
- Frontends pin pnpm and use frozen lockfile Docker installs.
- Added `.dockerignore` to keep frontend/build junk and source corpora out of the Railway backend image.

## Learning Center Vocabulary Manager

New Center route:

```text
/vocabulary-manager
```

Features:

- check up to 200 English entries for global existence;
- add a verified missing EN→UZ lexeme;
- bulk add up to 100 rows;
- backend validation and duplicate prevention;
- center/user contribution audit history.

## Retained V5 features

- dictionary search-triggered spaced repetition;
- adaptive Again/Hard/Good/Easy vocabulary review;
- points/analytics integration;
- private listening audio controls;
- English and SAT assessment workflows;
- monochrome light/dark theme across all portals;
- previously fixed Daily Vocabulary PostgreSQL date arithmetic and Center Listening UI issues.

## Validation

`python tools/qa_v5.py` passes with zero repository issues. Dependency-backed Go/Next builds could not run in the packaging sandbox because external dependency registries were unreachable; run them in Railway/Vercel CI or a connected local environment before final traffic cutover.
