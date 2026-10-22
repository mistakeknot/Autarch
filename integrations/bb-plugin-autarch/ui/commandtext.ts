// A card filed as free text carries its commands as prose. This splits a question into prose and command lines so
// each command can be one copyable block (bead mk-yjp7). Pure: nothing is run, and a command's text is returned
// exactly as written (only a leading "$ " prompt is dropped, because it is not part of the command).
export type CommandRole = "check" | "step" | "final" | "undo";
export type TextPart = { type: "prose"; text: string } | { type: "command"; text: string; role: CommandRole };

const COMMAND_WORDS = ["bash", "sudo", "runuser", "scp", "ssh", "sh", "python3", "gh", "bb", "bd", "autarch", "sonnerie"];
const DRY_RUN = /(^|\s)--(check|dry-run)(\s|=|$)/;
/** An undo flag anywhere, or a rollback/undo word as the first argument after the program or script. Quoted text and other flags' values do not count. */
const UNDO_FLAG = /(^|\s)--(rollback|undo|revert)(\s|=|$)/;
const UNDO_SUBCOMMAND = /^(?:sudo(?:\s+-\S+(?:\s+[^\s-]\S*)?)*\s+|runuser\s+-u\s+\S+\s+--\s+)*(?:(?:bash|sh|python3)\s+)?\S+\s+(?:rollback|undo|revert)(\s|$)/;
const isUndoCommand = (text: string): boolean => {
  const bare = text.replace(/"(?:\\.|[^"\\])*"|'[^']*'/g, '""');
  return UNDO_FLAG.test(bare) || UNDO_SUBCOMMAND.test(bare);
};
/** The prose line right before a command opens by naming it the undo ("Undo:", "Rollback, only if needed:", "Roll back if it goes wrong:"). */
const UNDO_LEAD = /^\W*(undo|roll ?back|revert)\b[^\n]*:\s*$/i;

const SENTENCE_START = /^(is|are|was|were|the|a|an|it|that|this|to|and|or|if|when|then)\b/i;

/** The last word reads as a file or path ("x.sh", "/tmp/o", "--out=/x"), so a closing period belongs to it. */
const PATH_LIKE = /[/\\=]|\w\.\w/;

// Words that make a line a sentence. Words that are also real arguments ("all", "list", "for", "with", "only") are left out.
const PROSE_WORDS = new Set(("the a an is are was were be been to of that this these those it its because when then if will can could should would may might has have had need needs must not does supports support reports shows prints returns outputs works means gives lets says uses helps").split(" "));

/** Quoted strings and the value right after a flag ("--label support") are arguments, whatever words they hold; any other sentence word makes a line prose. */
function readsAsSentence(args: string): boolean {
  const tokens = args.replace(/"[^"]*"|'[^']*'/g, " ").split(/\s+/).filter((w) => w !== "");
  return tokens.some((w, i) => /^[A-Za-z]+$/.test(w) && PROSE_WORDS.has(w.toLowerCase()) && !(i > 0 && tokens[i - 1]!.startsWith("-")));
}

/** Leading space and line-end whitespace go, but a space escaped by a backslash belongs to the argument and stays. */
function trimLine(line: string, keepIndent = false): string {
  const s = line.replace(/\r$/, "").replace(keepIndent ? /^$/ : /^\s+/, "");
  const m = /\s+$/.exec(s);
  if (!m) return s;
  const head = s.slice(0, m.index);
  const escapes = /\\*$/.exec(head)![0].length;
  return escapes % 2 === 1 ? s.slice(0, m.index + 1) : head;
}

/** A command line: a "$ " prompt followed by anything, or a known command word with an argument that does not read as a sentence. */
function commandOf(line: string): string | null {
  const t = trimLine(line);
  if (t.startsWith("$ ")) {
    const typed = trimLine(t.slice(2));
    return typed === "" ? null : typed; // the prompt says it is a command; any program is allowed
  }
  const m = /^(\S+)\s+(\S.*)$/.exec(t);
  if (!m || !COMMAND_WORDS.includes(m[1]!) || SENTENCE_START.test(m[2]!) || readsAsSentence(m[2]!)) return null;
  // A sentence ends in punctuation after a plain word ("bb supports --json output."); a command ends in a path or a flag value.
  const last = t.split(/\s+/).pop()!;
  if (/[.!?,:]$/.test(t) && !PATH_LIKE.test(last) && /[A-Za-z0-9]/.test(last)) return null; // a bare "." or "-" is an argument
  return t;
}

const LIST_MARKER = /^\s*(?:\(\d{1,3}\)|\d{1,3}[.)])\s+/;
/** A trailing note in square brackets that holds words and opens right at its text ("[read-only: one GET]"). A shell test ("[ -f x ]", "[[ ... ]]") opens with a space or a second bracket, and one without a space ("[a-z]") is an argument. */
const MAX_NOTE_LINE = 2000; // longer lines skip note splitting, which keeps the match cheap
const TRAILING_NOTE = /\s+(\[[^\[\]\s\\'"][^\[\]\\'"]*\s[^\[\]\\'"]*\])\s*$/; // no backslash or quote inside: "[a\ b]" and '[a" "b]' are arguments
const ENDS_IN_ESCAPE = /(?:^|[^\\])(?:\\\\)*\\$/;
const ENDS_IN_OPERATOR = /(?:&&|\|\||[|;&])\s*$/;

/** True when every quote in the text is closed, so a bracket after it is outside any quoted argument. */
function quotesClosed(text: string): boolean {
  let quote = "";
  for (let i = 0; i < text.length; i++) {
    const c = text[i]!;
    if (quote !== "") {
      if (c === "\\" && quote === '"') i++;
      else if (c === quote) quote = "";
    } else if (c === "'" || c === '"') quote = c;
    else if (c === "\\") i++;
  }
  return quote === "";
}

/** A command line, allowing a list number in front ("(1) ", "1. ") and a bracketed note after. Neither is part of the command, so it is split off before the line is judged. */
function listedCommandOf(line: string): { command: string; note: string | null } | null {
  const body = line.replace(LIST_MARKER, "");
  const m = body.length > MAX_NOTE_LINE ? null : TRAILING_NOTE.exec(body);
  if (m && !ENDS_IN_OPERATOR.test(body.slice(0, m.index)) && !ENDS_IN_ESCAPE.test(body.slice(0, m.index)) && quotesClosed(body.slice(0, m.index))) {
    const command = commandOf(body.slice(0, m.index));
    if (command !== null) return { command, note: m[1]! };
  }
  const command = commandOf(body);
  return command === null ? null : { command, note: null };
}

/** Where an inline command ends: before a note in parentheses or a sentence end, outside quotes. A "(" right after an operator ("&& (cd x)") opens a subshell, and a period after a space ("--out .") is an argument. */
function inlineEnd(rest: string, atParen = true): number {
  let quote = "";
  for (let i = 0; i < rest.length; i++) {
    const c = rest[i]!;
    if (quote !== "") {
      if (c === "\\" && quote === '"') i++;
      else if (c === quote) quote = "";
    } else if (c === "'" || c === '"') quote = c;
    else if (c === "\\") i++;
    else if (atParen && c === "(" && i > 0 && /\s/.test(rest[i - 1]!) && !/(?:&&|\|\||[|;&])\s*$/.test(rest.slice(0, i))) return i;
    else if (c === "." && i > 0 && /[^\s.]/.test(rest[i - 1]!) && (i + 1 === rest.length || /^\s+(?:[A-Z]|$)/.test(rest.slice(i + 1)))) return i;
  }
  return rest.length;
}

/** A command written inside a sentence after a colon ("paste one line: scp host:/p /tmp/x && bash /tmp/x (sha256 ...). It builds"). */
function inlineOf(line: string): { before: string; command: string; after: string } | null {
  for (const m of line.matchAll(/:\s+(?=(\S+)\s+\S)/g)) {
    if (!COMMAND_WORDS.includes(m[1]!)) continue;
    const start = m.index! + m[0].length;
    const rest = line.slice(start);
    const end = inlineEnd(rest);
    const command = trimLine(rest.slice(0, end));
    if (commandOf(command) !== command) continue;
    // A command has something that only a command has (a path, flag, operator or variable); "bash scripts" alone is a phrase.
    // A flag, operator, "=", variable or host path anywhere counts; a plain file or path counts only as the first argument ("bash x.sh"), so "ssh keys in /srv/k.pub" stays prose.
    const args = command.slice(command.indexOf(" ") + 1);
    if (!/[=|&;$]|\w:\/|(?:^|\s)-/.test(args) && !/^(?:\S*\/\S*|\S*\w\.\w\S*)(?:\s|$)/.test(args)) continue;
    // The whole sentence must not read as prose either ("Note: bash scripts (including this one) need review."): a note in parentheses is not part of the command but is part of that judgement.
    // A chained command or a host path is a command whatever its note says ("(sha256 must be ...)"), so only the command's own words are judged then.
    const chained = /&&|\|\||[|;]|\w:\/\S/.test(args);
    if (readsAsSentence(rest.slice(command.length === 0 ? 0 : rest.indexOf(" ") + 1, inlineEnd(rest, chained)))) continue;
    return { before: line.slice(0, start).replace(/\s+$/, ""), command, after: rest.slice(end).replace(/^\s+/, "") };
  }
  return null;
}

/** A command that is continued onto the next line with a trailing backslash is one command. */
const CONTINUES = /(?:^|[^\\])(?:\\\\)*\\$/; // an odd run of backslashes right at the end: the newline is escaped

export function splitCommands(text: string): TextPart[] {
  const parts: TextPart[] = [];
  let prose: string[] = [];
  const flush = () => {
    const joined = prose.join("\n").replace(/^\n+|\n+$/g, "");
    if (joined !== "") parts.push({ type: "prose", text: joined });
    prose = [];
  };
  const leads = new Map<TextPart, string>(); // the last prose line before each command
  const leadLine = () => [...prose].reverse().find((l) => l.trim() !== "") ?? "";
  const lines = text.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const listed = listedCommandOf(lines[i]!);
    const c = listed === null ? null : listed.command;
    if (c === null) {
      const inline = inlineOf(lines[i]!);
      if (inline === null) {
        prose.push(lines[i]!);
        continue;
      }
      prose.push(inline.before);
      const lead = leadLine();
      flush();
      const part: TextPart = { type: "command", text: inline.command, role: "step" };
      leads.set(part, lead);
      parts.push(part);
      if (inline.after !== "") prose.push(inline.after);
      continue;
    }
    const lead = leadLine();
    flush();
    const whole = [c];
    while (CONTINUES.test(whole[whole.length - 1]!) && i + 1 < lines.length) whole.push(trimLine(lines[++i]!, true));
    const part: TextPart = { type: "command", text: whole.join("\n"), role: "step" };
    leads.set(part, lead);
    parts.push(part);
    if (listed!.note !== null) prose.push(listed!.note);
  }
  flush();
  const cmds = parts.filter((p): p is Extract<TextPart, { type: "command" }> => p.type === "command");
  // Roles follow what a command does, not where it stands: the last command that is neither a check nor an undo is the one to run.
  // A lone command is always the final one, whatever it is.
  const kind = (c: Extract<TextPart, { type: "command" }>): CommandRole =>
    isUndoCommand(c.text) || UNDO_LEAD.test(leads.get(c) ?? "") ? "undo" : DRY_RUN.test(c.text) ? "check" : "step";
  cmds.forEach((c) => { c.role = cmds.length === 1 ? "final" : kind(c); });
  const run = cmds.filter((c) => c.role === "step").pop();
  if (run) run.role = "final";
  return parts;
}

/** The `--flag` names in a command, in order, without their values. */
export function flagsOf(command: string): string[] {
  const out: string[] = [];
  for (const tok of command.split(/\s+/)) {
    const m = /^(--[A-Za-z][\w-]*)(?:=.*)?$/.exec(tok);
    if (m) out.push(m[1]!);
  }
  return out;
}
