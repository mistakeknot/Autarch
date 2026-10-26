// The document reader's Markdown: a fuller block grammar than the card-text subset in markdown.tsx (which stays small
// on purpose). Adds headings h1-h6, tables, numbered and nested lists, blockquotes and horizontal rules. The same
// safety rules hold: every node is a React element built from parsed text, never HTML; only http(s) links become
// anchors; images are never fetched. Inline spans (code, bold, links) come from the card-text parser.
// Prose is set in the body font; only code is monospace.
import type { ReactNode } from "react";
import { inline, MARKDOWN_MAX_CHARS } from "./markdown.js";

const MAX_DEPTH = 6;
const HEADING = /^ {0,3}(#{1,6})[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$/;
const RULE = /^ {0,3}([-*_])(?:[ \t]*\1){2,}[ \t]*$/;
const ITEM = /^(\s*)([-*+]|\d{1,9}[.)])[ \t]+(.*)$/;
const SEP_CELL = /^:?-+:?$/;
const HEADING_CLASS = ["text-2xl", "text-xl", "text-lg", "text-base", "text-sm", "text-sm"];

type Item = { indent: number; ordered: boolean; text: string; kids: Item[]; tables: string[][] };

function cells(line: string): string[] {
  let s = line.trim();
  if (s.startsWith("|")) s = s.slice(1);
  if (s.endsWith("|") && !s.endsWith("\\|")) s = s.slice(0, -1);
  return s.split(/(?<!\\)\|/).map((c) => c.trim().replace(/\\\|/g, "|"));
}

// Block patterns run on at most this many characters of a line, so no pattern can be driven quadratic by one long line.
const MAX_LINE = 2000;
const MAX_COLS = 50;
/** Above this size the whole document is shown as plain wrapped text, unparsed. */
const MAX_DOC_CHARS = MARKDOWN_MAX_CHARS * 5;
const t = (l: string) => (l.length > MAX_LINE ? "x" : l);

// Each cell is checked on its own after a linear split, so no pattern scans the same whitespace twice.
function isSeparator(line: string): boolean {
  return cells(line).every((c) => SEP_CELL.test(c));
}

function startsTable(line: string, next: string | undefined): boolean {
  return line.length <= MAX_LINE && cells(line).length <= MAX_COLS && line.includes("|") && next !== undefined && next.length <= MAX_LINE && next.includes("-") && isSeparator(next) && cells(next).length === cells(line).length;
}

function startsBlock(line: string, next: string | undefined): boolean {
  return (
    line.startsWith("```") ||
    HEADING.test(t(line)) ||
    RULE.test(t(line)) ||
    /^ {0,3}>/.test(line) ||
    ITEM.test(t(line)) ||
    startsTable(line, next)
  );
}

/** Items past the nesting limit are shown as plain indented lines rather than dropped. */
function flatten(items: Item[], level: number, out: string[] = []): string[] {
  for (const it of items) {
    out.push(`${"  ".repeat(level)}${it.ordered ? "" : "- "}${it.text}`);
    flatten(it.kids, level + 1, out);
  }
  return out;
}

/** One list per run of the same marker type, so "- a" followed by "1. b" are two lists. */
function renderItems(items: Item[], key: string, depth: number): ReactNode[] {
  const runs: Item[][] = [];
  for (const it of items) {
    const last = runs[runs.length - 1];
    if (last && last[0].ordered === it.ordered) last.push(it);
    else runs.push([it]);
  }
  return runs.map((run, r) => {
    const Tag = run[0].ordered ? "ol" : "ul";
    return (
      <Tag key={`${key}-${r}`} className={`m-0 flex flex-col gap-0.5 pl-6 ${run[0].ordered ? "list-decimal" : "list-disc"}`}>
        {run.map((it, n) => (
          <li key={n}>
            {inline(it.text, `${key}-${r}-${n}`)}
            {it.tables.map((tb, k) => <div key={`t${k}`} className="my-1">{blocks(tb, depth + 1)}</div>)}
            {it.kids.length === 0 ? null : depth < MAX_DEPTH ? renderItems(it.kids, `${key}-${r}-${n}k`, depth + 1) : <div className="whitespace-pre-wrap">{inline(flatten(it.kids, 0).join("\n"), `${key}-${r}-${n}f`)}</div>}
          </li>
        ))}
      </Tag>
    );
  });
}

/** Group consecutive item lines into a tree by indent; an indented non-item line continues the previous item. */
function readList(lines: string[], start: number): { items: Item[]; next: number } {
  const roots: Item[] = [];
  const stack: Item[] = [];
  let i = start;
  while (i < lines.length) {
    const m = ITEM.exec(t(lines[i]));
    if (m && !RULE.test(lines[i])) {
      const it: Item = { indent: m[1].replace(/\t/g, "    ").length, ordered: /\d/.test(m[2]), text: m[3], kids: [], tables: [] };
      while (stack.length > 0 && stack[stack.length - 1].indent >= it.indent) stack.pop();
      (stack.length > 0 ? stack[stack.length - 1].kids : roots).push(it);
      stack.push(it);
      i++;
    } else if (lines[i].trim() !== "" && /^\s+/.test(lines[i]) && stack.length > 0 && startsTable(lines[i].trim(), lines[i + 1]?.trim())) {
      // An indented table belongs to the item above it, so the list carries on after it.
      const table = [lines[i].trim(), lines[i + 1].trim()];
      i += 2;
      while (i < lines.length && /^\s+/.test(lines[i]) && lines[i].includes("|") && lines[i].length <= MAX_LINE && !startsBlock(lines[i].trim(), undefined)) table.push(lines[i++].trim());
      stack[stack.length - 1].tables.push(table);
    } else if (lines[i].trim() !== "" && /^\s+/.test(lines[i]) && stack.length > 0 && !startsBlock(lines[i].trim(), lines[i + 1]?.trim())) {
      stack[stack.length - 1].text += `\n${lines[i].trim()}`;
      i++;
    } else break;
  }
  return { items: roots, next: i };
}

function blocks(lines: string[], depth: number): ReactNode[] {
  const out: ReactNode[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    const key = out.length;
    let m: RegExpExecArray | null;
    if (line.startsWith("```")) {
      const body: string[] = [];
      const ticks = /^`+/.exec(line)![0].length;
      i++;
      // A closing fence is a bare line of at least as many backticks as the opening one.
      while (i < lines.length && !(/^`+[ \t]*$/.test(lines[i]) && lines[i].trim().length >= ticks)) body.push(lines[i++]);
      i++;
      out.push(<pre key={key} className="m-0 overflow-x-auto rounded bg-muted p-2 font-mono text-xs"><code>{body.join("\n")}</code></pre>);
    } else if (line.trim() === "") {
      i++;
    } else if ((m = HEADING.exec(t(line)))) {
      const level = m[1].length;
      const Tag = `h${level}` as "h1";
      out.push(<Tag key={key} className={`m-0 mt-2 font-semibold ${HEADING_CLASS[level - 1]}${level <= 2 ? " border-b border-border pb-1" : ""}`}>{inline(m[2], String(key))}</Tag>);
      i++;
    } else if (RULE.test(t(line))) {
      out.push(<hr key={key} className="my-2 border-border" />);
      i++;
    } else if (/^ {0,3}>/.test(line)) {
      const body: string[] = [];
      while (i < lines.length && /^ {0,3}>/.test(lines[i])) body.push(lines[i++].replace(/^ {0,3}> ?/, ""));
      out.push(
        <blockquote key={key} className="m-0 flex flex-col gap-1 border-l-4 border-border pl-3 text-muted-foreground">
          {depth < MAX_DEPTH ? blocks(body, depth + 1) : <p className="m-0 whitespace-pre-wrap">{body.join("\n")}</p>}
        </blockquote>,
      );
    } else if (ITEM.test(t(line))) {
      const { items, next } = readList(lines, i);
      i = next;
      out.push(<div key={key} className="flex flex-col gap-2">{renderItems(items, String(key), 0)}</div>);
    } else if (startsTable(line, lines[i + 1])) {
      const head = cells(line);
      const align = cells(lines[i + 1]).map((c) => (c.startsWith(":") && c.endsWith(":") ? "center" : c.endsWith(":") ? "right" : "left"));
      i += 2;
      const rows: string[][] = [];
      while (i < lines.length && lines[i].trim() !== "" && lines[i].includes("|") && lines[i].length <= MAX_LINE && !startsBlock(lines[i], undefined)) rows.push(cells(lines[i++]));
      const style = (c: number) => ({ textAlign: (align[c] ?? "left") as "left" | "right" | "center" });
      out.push(
        <div key={key} className="overflow-x-auto">
          <table className="border-collapse text-sm">
            <thead>
              <tr>{head.map((h, c) => <th key={c} style={style(c)} className="border border-border bg-muted px-2 py-1 font-semibold">{inline(h, `${key}h${c}`)}</th>)}</tr>
            </thead>
            <tbody>
              {rows.map((r, n) => (
                <tr key={n}>{r.slice(0, head.length).map((cell, c) => <td key={c} style={style(c)} className="border border-border px-2 py-1 align-top">{inline(cell, `${key}r${n}c${c}`)}</td>)}</tr>
              ))}
            </tbody>
          </table>
        </div>,
      );
    } else {
      const para: string[] = [];
      while (i < lines.length && lines[i].trim() !== "" && (para.length === 0 || !startsBlock(lines[i], lines[i + 1]))) para.push(lines[i++]);
      out.push(<p key={key} className="m-0 whitespace-pre-wrap">{inline(para.join("\n"), String(key))}</p>);
    }
  }
  return out;
}

export function DocMarkdown({ text }: { text: string }) {
  if (text.length > MAX_DOC_CHARS) return <p className="m-0 whitespace-pre-wrap font-sans text-sm" data-markdown-plain>{text}</p>;
  return <div className="flex min-w-0 flex-col gap-2 font-sans text-sm leading-6 [overflow-wrap:anywhere]" data-doc-markdown>{blocks(text.replace(/\r\n?/g, "\n").split("\n"), 0)}</div>;
}
