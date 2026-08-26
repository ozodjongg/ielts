# Vocabulary data

Production uses clean lexical data only.

- `demo_seed.csv` / `demo_synonyms.csv`: small clean bootstrap dataset.
- `generated/`: clean collector output; `stage1_import_ready.csv` is preferred for production auto-import.
- `legacy_opus/`: historical 100K OPUS-derived corpus. It contains mostly phrases/sentences and is deliberately excluded from automatic import.

Generate clean data with:

```bash
python tools/collect_vocabulary_stage1.py --root .
```

Then deploy/redeploy. Docker will import a new dataset version once.
