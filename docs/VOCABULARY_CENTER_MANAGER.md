# Learning Center Vocabulary Manager

Center portal route:

```text
/vocabulary-manager
```

This page lets trusted `center_admin` users expand the shared EN→UZ vocabulary without creating duplicate English headwords.

## UI workflow

1. **Check missing words** — paste up to 200 English words/short lexical phrases, one per line.
2. **Add one word** — enter English, one or more Uzbek translations, CEFR and POS.
3. **Bulk add** — up to 100 lines using:

```text
English | Uzbek translation(s) | CEFR | POS
```

Examples:

```text
achieve | erishmoq, qo‘lga kiritmoq | B1 | verb
accurate | aniq | B2 | adjective
look after | g‘amxo‘rlik qilmoq | A2 | phrasal_verb
```

4. **Contribution history** — shows words added by the current Learning Center.

## Backend routes

```text
POST /v1/center/words/check
POST /v1/center/words
POST /v1/center/words/batch
GET  /v1/center/contributions
```

The gateway exposes these through `/api/center/vocabulary/...`.

## Backend-enforced rules

- only authenticated `center_admin` users with an organization can write;
- global `normalized_english` is checked before insert;
- concurrent center inserts for the same normalized English value are serialized with a PostgreSQL transaction advisory lock;
- English is limited to a lexical word/short phrase, maximum 4 words;
- digits and sentence-heavy punctuation are rejected;
- each Uzbek translation must be Latin-script lexical text, maximum 6 words;
- 1–12 Uzbek translations per entry;
- CEFR is restricted to A1/A2/B1/B2/C1/C2;
- POS is restricted to the supported controlled list;
- batch size is maximum 100;
- contribution provenance stores organization, user, lexeme and timestamp.

Center-added data is globally visible to the shared dictionary. Learning Centers should add only reviewed translations and content they are allowed to use.
