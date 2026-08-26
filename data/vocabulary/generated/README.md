# Clean production vocabulary output

Place the clean lexical dataset here before deployment.

Preferred files:

- `stage1_import_ready.csv` — clean EN→UZ lexemes with official CEFR labels
- `stage1_synonyms_import_ready.csv` — optional synonym edges where both words exist in the imported lexeme set

The production Docker entrypoint imports `stage1_import_ready.csv` once when it is present. If it is not present, the container falls back to the small clean `demo_seed.csv` so a fresh deployment remains usable.

The legacy OPUS corpus is intentionally kept under `../legacy_opus/` and is **not** auto-imported because it contains sentences and noisy aligned expressions rather than a curated dictionary.
