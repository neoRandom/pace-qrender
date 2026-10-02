# qrender

Renders the JSON question records written by the exam-conversion agent into the Markdown question format. Go standard library only.

    go build -o qrender .
    ./qrender -o md questions.zip        # or a folder containing questions/ and images/

## Input

`questions.zip` (or a folder) with `questions/<NNN>-<slug>.json` and `images/*`. A zip with one extra top-level folder is accepted.

## What it does per file

1. Decodes strictly: exactly the ten keys (`slug category tags source effort knowledge answer content alternatives images`), right types.
2. Validates (errors block the run, warnings do not):
   - slug: 2–6 lowercase ASCII words; knowledge: `None` or `~10`, `~100`, …; `answer` in range; at least 2 alternatives.
   - metadata single-line and trimmed; tags contain no `;`.
   - every block: no blank line outside a code fence, balanced fences, no HTML comment, blockquote lines all start with `>`, no unescaped `R$ 12` or `US$ 12`, even number of unescaped `$`, well-formed `![label][imageN]`.
   - images numbered `image1…` in order of appearance, listed in `images` in the same order, paths under `images/`.
   - warnings: alternative starting with a label (`A)`, `1.`), table without separator row, heading-like or list-like line, empty tags, image over 1.5 MiB, `file: null` image.
3. Embeds each image as `data:<mime>;base64,…` (type detected from the bytes: PNG, JPEG, GIF, WebP). `file: null` writes `[imageN]: PLACEHOLDER`.
4. Writes `<slug>-<id>.md`: front matter (`tags` joined with `; `; values double-quoted only when YAML needs it), `# Content`, `# Alternatives`, then image definitions. Blocks are separated by one blank line.

## Identifiers

Default: first 6 hex characters of SHA-256(`source` NUL `slug`). Re-running gives the same file names, so output can be overwritten or diffed. `-random` uses random hex (checked against files already in the output folder). `-hash-len N` changes the length (4–64). A name collision between different questions lengthens that identifier by 2 characters. Two records with the same `source` and `slug` are an error.

## Flags and exit codes

    -o dir       output folder (default md)
    -strict      warnings, including PLACEHOLDER images, become errors
    -partial     write the valid questions even if others fail
    -map file    write a TSV: JSON file name -> Markdown file name
    -random, -hash-len N

All-or-nothing by default: if any file has an error, nothing is written. Exit 0 on success, 1 on validation errors, 2 on usage or I/O errors.

## Tests

    go test ./...

`examples/` holds four records from the prompt (table, verse with citation, image with currency and math, chemistry with table alternatives): `./qrender -o md examples`.
