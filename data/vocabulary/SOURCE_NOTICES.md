# Source notices

## OPUS-100 English–Uzbek v1.0

This package was derived from the aligned English–Uzbek training files distributed by OPUS-100:

- Project page: https://opus.nlpl.eu/OPUS-100
- English source file: `opus.en-uz-train.en`
- Uzbek source file: `opus.en-uz-train.uz`

OPUS-100 is a multilingual parallel corpus assembled from multiple upstream OPUS corpora.
The upstream texts and their licensing terms are heterogeneous. OPUS states that it distributes
files it believes it is free to redistribute and provides a notice-and-takedown process.

Before commercial redistribution of the derived data, review the OPUS project terms and the
upstream corpus licensing requirements applicable to your use case.

Recommended academic attribution:
- Jörg Tiedemann (2012), "Parallel Data, Tools and Interfaces in OPUS."
- Biao Zhang, Philip Williams, Ivan Titov, Rico Sennrich (2020),
  "Improving Massively Multilingual Neural Machine Translation and Zero-Shot Translation."

## Important data-quality note

This is a 100,000-entry *learning vocabulary/expression corpus*, not a human-edited 100,000-word
dictionary. It contains:
- single English words,
- short phrases,
- and aligned expressions/sentences.

Uzbek Cyrillic source text was deterministically transliterated to Latin script where present.
No machine translation was invented to fill missing entries.

CEFR values are frequency/complexity heuristics created for the V5 learning workflow and are not
official CEFR annotations.
