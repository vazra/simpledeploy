/**
 * gen-cli.ts
 *
 * Walks `simpledeploy --help` to emit Starlight markdown reference pages
 * under docs/reference/cli/. Designed to be safe in CI when the binary
 * has not been built: if no binary is found, it logs a warning and exits 0
 * so the docs build can still run.
 *
 * Run with: `pnpm run gen:cli` (uses tsx).
 */

import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// ---- locations -------------------------------------------------------------

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(HERE, "..", "..");
const OUT_DIR = join(REPO_ROOT, "docs", "reference", "cli");

// ---- types -----------------------------------------------------------------

interface Flag {
  short?: string;
  long?: string;
  type?: string;
  default?: string;
  description: string;
}

interface CommandDoc {
  path: string[];           // e.g. ["simpledeploy", "apps", "list"]
  short: string;
  long: string;
  usage: string;
  flags: Flag[];
  globalFlags: Flag[];
  examples: string;         // raw text block, may be empty
  subcommands: string[];    // names of immediate children
}

// ---- binary discovery ------------------------------------------------------

function findBinary(): string | null {
  const candidates = [
    join(REPO_ROOT, "bin", "simpledeploy"),
    join(REPO_ROOT, "simpledeploy"),
  ];
  for (const c of candidates) {
    if (existsSync(c)) return c;
  }
  // Fall back to PATH lookup
  try {
    const out = execFileSync("which", ["simpledeploy"], { encoding: "utf8" })
      .trim();
    if (out && existsSync(out)) return out;
  } catch {
    // ignored
  }
  return null;
}

// ---- help runner -----------------------------------------------------------

function runHelp(bin: string, args: string[]): string {
  try {
    return execFileSync(bin, [...args, "--help"], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    });
  } catch (err: unknown) {
    // Cobra returns non-zero when --help is given to a parent that has
    // no run function; the help text still goes to stdout/stderr.
    const e = err as { stdout?: string; stderr?: string };
    return (e.stdout ?? "") + (e.stderr ?? "");
  }
}

// ---- parser ----------------------------------------------------------------

const SECTIONS = [
  "Usage:",
  "Aliases:",
  "Examples:",
  "Available Commands:",
  "Flags:",
  "Global Flags:",
  "Additional Commands:",
  "Use \"",
];

function splitSections(help: string): Record<string, string> {
  const lines = help.split(/\r?\n/);
  const out: Record<string, string> = { _preamble: "" };
  let current = "_preamble";
  const buf: Record<string, string[]> = { _preamble: [] };
  for (const line of lines) {
    const trimmed = line.trimEnd();
    const matchedHeader = SECTIONS.find((s) => trimmed === s || trimmed.startsWith(s));
    if (matchedHeader && (matchedHeader.endsWith(":") || matchedHeader === "Use \"")) {
      current = matchedHeader.replace(/:$/, "").trim();
      buf[current] = [];
      continue;
    }
    buf[current] ??= [];
    buf[current].push(line);
  }
  for (const k of Object.keys(buf)) {
    out[k] = buf[k].join("\n").replace(/^\n+|\n+$/g, "");
  }
  return out;
}

function parseSubcommands(block: string | undefined): string[] {
  if (!block) return [];
  const names: string[] = [];
  for (const line of block.split(/\r?\n/)) {
    const m = line.match(/^\s{2,}([A-Za-z][A-Za-z0-9_-]*)\s{2,}/);
    if (m) names.push(m[1]);
  }
  return names;
}

const FLAG_RE = /^\s*(-[A-Za-z],\s)?(--[A-Za-z0-9_-]+)(?:\s+(\S+))?\s+(.*)$/;

function parseFlags(block: string | undefined): Flag[] {
  if (!block) return [];
  const flags: Flag[] = [];
  let last: Flag | null = null;
  for (const line of block.split(/\r?\n/)) {
    if (!line.trim()) continue;
    const m = line.match(FLAG_RE);
    if (m) {
      if (last) flags.push(last);
      const short = m[1]?.trim().replace(/,$/, "");
      const long = m[2];
      const type = m[3];
      let desc = m[4] ?? "";
      let def: string | undefined;
      const defMatch = desc.match(/\(default\s+(.*)\)\s*$/);
      if (defMatch) {
        def = defMatch[1].trim();
        desc = desc.slice(0, defMatch.index).trim();
      }
      last = { short, long, type, default: def, description: desc.trim() };
    } else if (last) {
      // Continuation line for previous flag's description
      last.description = `${last.description} ${line.trim()}`.trim();
    }
  }
  if (last) flags.push(last);
  return flags;
}

function parseHelp(help: string, path: string[]): CommandDoc {
  const sections = splitSections(help);
  const preambleLines = (sections._preamble ?? "").split(/\r?\n/);
  const short = (preambleLines[0] ?? "").trim();
  const long = preambleLines.slice(1).join("\n").trim() || short;
  return {
    path,
    short,
    long,
    usage: (sections["Usage"] ?? "").trim(),
    flags: parseFlags(sections["Flags"]),
    globalFlags: parseFlags(sections["Global Flags"]),
    examples: (sections["Examples"] ?? "").trim(),
    subcommands: parseSubcommands(sections["Available Commands"]),
  };
}

// ---- recursion -------------------------------------------------------------

function harvest(bin: string, path: string[], acc: CommandDoc[]): void {
  // path[0] is the binary name; arguments to the binary are path.slice(1)
  const args = path.slice(1);
  const help = runHelp(bin, args);
  const doc = parseHelp(help, path);
  acc.push(doc);
  for (const sub of doc.subcommands) {
    if (sub === "help" || sub === "completion") continue;
    harvest(bin, [...path, sub], acc);
  }
}

// ---- markdown emit ---------------------------------------------------------

function slugForPath(path: string[]): string {
  // Top-level binary => "index"; deeper => joined with dashes
  if (path.length === 1) return "index";
  return path.slice(1).join("-");
}

function flagsTable(flags: Flag[]): string {
  if (!flags.length) return "";
  const rows = flags.map((f) => {
    const name = [f.short, f.long].filter(Boolean).join(", ");
    const type = f.type ?? "";
    const def = f.default ?? "";
    const desc = f.description.replace(/\|/g, "\\|");
    return `| \`${name}\` | ${type} | ${def} | ${desc} |`;
  });
  return [
    "| Flag | Type | Default | Description |",
    "| ---- | ---- | ------- | ----------- |",
    ...rows,
  ].join("\n");
}

function renderPage(doc: CommandDoc): string {
  const title = doc.path.join(" ");
  const description = doc.short || `Reference for ${title}.`;
  const parts: string[] = [];
  parts.push("---");
  parts.push(`title: ${JSON.stringify(title)}`);
  parts.push(`description: ${JSON.stringify(description)}`);
  parts.push("---");
  parts.push("");
  parts.push("## Synopsis");
  parts.push("");
  parts.push("```bash");
  parts.push(doc.usage || title);
  parts.push("```");
  parts.push("");
  parts.push("## Description");
  parts.push("");
  parts.push(doc.long || doc.short || "_No description provided._");
  parts.push("");
  if (doc.flags.length) {
    parts.push("## Flags");
    parts.push("");
    parts.push(flagsTable(doc.flags));
    parts.push("");
  }
  if (doc.globalFlags.length) {
    parts.push("## Global Flags");
    parts.push("");
    parts.push(flagsTable(doc.globalFlags));
    parts.push("");
  }
  if (doc.examples) {
    parts.push("## Examples");
    parts.push("");
    parts.push("```bash");
    parts.push(doc.examples);
    parts.push("```");
    parts.push("");
  }
  if (doc.subcommands.length) {
    parts.push("## Subcommands");
    parts.push("");
    for (const sub of doc.subcommands) {
      if (sub === "help" || sub === "completion") continue;
      const slug = slugForPath([...doc.path, sub]);
      parts.push(`- [\`${doc.path.join(" ")} ${sub}\`](./${slug}.md)`);
    }
    parts.push("");
  }
  return parts.join("\n");
}

function renderIndex(docs: CommandDoc[]): string {
  const root = docs.find((d) => d.path.length === 1);
  const title = "CLI Reference";
  const parts: string[] = [];
  parts.push("---");
  parts.push(`title: ${JSON.stringify(title)}`);
  parts.push(`description: ${JSON.stringify("Auto-generated reference for every simpledeploy CLI command.")}`);
  parts.push("---");
  parts.push("");
  if (root) {
    parts.push(root.long || root.short);
    parts.push("");
  }
  // Group by top-level command
  const topLevel = docs
    .filter((d) => d.path.length === 2)
    .sort((a, b) => a.path[1].localeCompare(b.path[1]));
  parts.push("## Commands");
  parts.push("");
  for (const top of topLevel) {
    const slug = slugForPath(top.path);
    parts.push(`### [\`${top.path.join(" ")}\`](./${slug}.md)`);
    parts.push("");
    if (top.short) {
      parts.push(top.short);
      parts.push("");
    }
    const subs = docs.filter(
      (d) => d.path.length === 3 && d.path[1] === top.path[1],
    );
    if (subs.length) {
      for (const sub of subs) {
        const subSlug = slugForPath(sub.path);
        const tail = sub.short ? ` — ${sub.short}` : "";
        parts.push(`- [\`${sub.path.join(" ")}\`](./${subSlug}.md)${tail}`);
      }
      parts.push("");
    }
  }
  return parts.join("\n");
}

// ---- main ------------------------------------------------------------------

function main(): void {
  const bin = findBinary();
  if (!bin) {
    console.warn(
      "[gen-cli] simpledeploy binary not found in bin/, repo root, or PATH. " +
        "Skipping CLI doc generation. Run `make build-go` then re-run.",
    );
    process.exit(0);
  }
  console.log(`[gen-cli] using binary: ${bin}`);

  const docs: CommandDoc[] = [];
  harvest(bin, ["simpledeploy"], docs);

  mkdirSync(OUT_DIR, { recursive: true });
  for (const doc of docs) {
    const slug = slugForPath(doc.path);
    const path = join(OUT_DIR, `${slug}.md`);
    writeFileSync(path, renderPage(doc), "utf8");
  }
  writeFileSync(join(OUT_DIR, "index.md"), renderIndex(docs), "utf8");
  console.log(`[gen-cli] wrote ${docs.length + 1} files to ${OUT_DIR}`);
}

main();
