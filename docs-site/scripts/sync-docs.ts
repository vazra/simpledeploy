/**
 * sync-docs.ts
 *
 * Mirrors repo /docs/**\/*.{md,mdx} into docs-site/src/content/docs/.
 * Site-only paths (landing, blog, community, license, playground) are
 * preserved. Everything else under those top-level section dirs is wiped
 * before sync to avoid stale placeholder files lingering.
 *
 * Skips /docs/superpowers/ (project planning material, not user docs).
 */
import { mkdir, readdir, readFile, writeFile, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, "..", "..");
const SRC = join(ROOT, "docs");
const DEST = join(__dirname, "..", "src", "content", "docs");

const EXTS = new Set([".md", ".mdx"]);
const SKIP_PREFIX = "superpowers";

// Top-level paths under DEST that must NOT be touched by sync.
// These are authored only inside docs-site (landing, blog, etc.).
const PROTECTED = new Set([
  "index.mdx",
  "index.md",
  "playground",
  "community",
  "blog",
  "license.md",
  "license.mdx",
  "changelog.md",
  "changelog.mdx",
]);

async function walk(dir: string, base: string): Promise<string[]> {
  const out: string[] = [];
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return out;
  }
  for (const e of entries) {
    const full = join(dir, e.name);
    const rel = relative(base, full);
    if (rel.split(/[\\/]/)[0] === SKIP_PREFIX) continue;
    if (e.isDirectory()) {
      out.push(...(await walk(full, base)));
    } else if (e.isFile()) {
      const dot = e.name.lastIndexOf(".");
      const ext = dot === -1 ? "" : e.name.slice(dot).toLowerCase();
      if (EXTS.has(ext)) out.push(full);
    }
  }
  return out;
}

/** Compute top-level section dirs in /docs (e.g. guides, reference). */
async function topLevelSections(): Promise<string[]> {
  let entries;
  try {
    entries = await readdir(SRC, { withFileTypes: true });
  } catch {
    return [];
  }
  const out: string[] = [];
  for (const e of entries) {
    if (e.name === SKIP_PREFIX) continue;
    if (PROTECTED.has(e.name)) continue;
    if (e.isDirectory()) out.push(e.name);
    else if (e.isFile() && EXTS.has(e.name.slice(e.name.lastIndexOf(".")).toLowerCase())) {
      // single-file top-level entries (e.g. faq.md) get cleaned + replaced
      out.push(e.name);
    }
  }
  return out;
}

async function wipeNonProtected(sections: string[]) {
  for (const s of sections) {
    const target = join(DEST, s);
    if (!existsSync(target)) continue;
    await rm(target, { recursive: true, force: true });
  }
}

async function main() {
  if (!existsSync(SRC)) {
    console.warn(`[sync-docs] /docs not found at ${SRC}; skipping.`);
    await mkdir(DEST, { recursive: true });
    return;
  }

  // Wipe top-level section dirs that /docs owns. Protected paths are kept.
  const sections = await topLevelSections();
  await wipeNonProtected(sections);

  const files = await walk(SRC, SRC);
  let copied = 0;
  for (const f of files) {
    const rel = relative(SRC, f);
    // Defense in depth: never write into protected paths.
    const top = rel.split(/[\\/]/)[0];
    if (PROTECTED.has(top)) continue;
    const target = join(DEST, rel);
    await mkdir(dirname(target), { recursive: true });
    const raw = await readFile(f, "utf8");
    const fixed = sanitizeFrontmatter(raw);
    await writeFile(target, fixed, "utf8");
    copied++;
  }
  console.log(`[sync-docs] copied ${copied} file(s) -> ${relative(process.cwd(), DEST)}`);
}

/**
 * Auto-quote unquoted YAML scalar values that contain a colon, which would
 * otherwise be misparsed as nested mappings. Only touches lines inside the
 * leading frontmatter block. No-op for files without frontmatter.
 */
function sanitizeFrontmatter(src: string): string {
  if (!src.startsWith("---")) return src;
  const end = src.indexOf("\n---", 3);
  if (end === -1) return src;
  const head = src.slice(0, end);
  const tail = src.slice(end);
  const lines = head.split("\n");
  for (let i = 1; i < lines.length; i++) {
    const m = lines[i].match(/^([A-Za-z_][\w-]*): (.+)$/);
    if (!m) continue;
    const [, key, val] = m;
    const trimmed = val.trim();
    if (/^["'\[\{]/.test(trimmed)) continue;
    if (/^(true|false|null|~|-?\d+(\.\d+)?)$/i.test(trimmed)) continue;
    if (/: |:$/.test(trimmed)) {
      const escaped = trimmed.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
      lines[i] = `${key}: "${escaped}"`;
    }
  }
  return lines.join("\n") + tail;
}

main().catch((err) => {
  console.error("[sync-docs] failed:", err);
  process.exit(0);
});
