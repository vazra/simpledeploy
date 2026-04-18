---
title: Writing docs
description: "House style for SimpleDeploy docs: markdown layout, components, screenshots, and cross-links."
---

The docs site is Astro Starlight. Markdown source lives in `docs/`. The site config and any interactive content live in `docs-site/`. A sync script copies `docs/**/*.md` into `docs-site/src/content/docs/` at build time.

## Source files

Every page starts with frontmatter:

```yaml
---
title: Sentence-case title
description: 60-160 char summary used in search and meta tags.
---
```

If the file used to start with `# Title`, remove that h1. Starlight renders the title from frontmatter.

## Tone

- Explain to a smart skeptical engineer.
- Be terse. Sacrifice grammar for brevity.
- Never use em dashes. Use commas, parens, or rewrite.
- Sentence case for headings, not Title Case.
- Imperative voice in procedures ("Run", "Open").

## Components

Import from `@astrojs/starlight/components`:

- `<Steps>` for numbered procedures.
- `<Tabs>` + `<TabItem>` for variants (UI vs CLI vs API, OS variants).
- `<Aside type="tip|caution|danger">` for callouts.
- `<CardGrid>` + `<Card>` for landing-style sections.
- `<FileTree>` for directory layouts.
- Mermaid via fenced ```mermaid blocks.

## Cross-links

Use absolute paths inside the docs (`/guides/tls/`). The site applies the base URL automatically when navigating.

## Screenshots

Live in `docs-site/public/screenshots/`. Reference with HTML or markdown:

```md
![App detail page in dark mode](/screenshots/appdetail-dark.png)
```

The image-zoom plugin opens them full size on click. Avoid screenshots of UI flows that change frequently; prefer text instructions.

## Code blocks

Specify the language for syntax highlighting. Common ones used: `yaml`, `bash`, `json`, `go`, `dockerfile`, `caddy`.

## Length

Most pages are 200-500 words. Reference pages can be longer (tables). If a page exceeds 1000 words, consider splitting.

## Local preview

```bash
cd docs-site
pnpm install
pnpm dev
```

Runs the docs at `http://localhost:4321/simpledeploy/`. Hot reload picks up edits to both `docs/` and `docs-site/`.
