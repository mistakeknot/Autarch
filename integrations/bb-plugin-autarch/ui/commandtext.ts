// A card filed as free text carries its commands as prose. This splits a question into prose and command lines so
// each command can be one copyable block (bead mk-yjp7). Pure: nothing is run, and a command's text is returned
// exactly as written (only a leading "$ " prompt is dropped, because it is not part of the command).
export type CommandRole = "check" | "step" | "final" | "undo";
export type TextPart = { type: "prose"; text: string } | { type: "command"; text: string; role: CommandRole };

const COMMAND_WORDS = ["bash", "sudo", "runuser", "scp", "ssh", "sh", "python3", "gh", "bb", "bd", "autarch", "sonnerie"];
/** The exact flags that make one simple command a check or an undo. A flag with a value ("--check=false") is neither. */
const CHECK_FLAG = /^--(?:check|dry-run)$/;
const UNDO_FLAG = /^--(?:rollback|undo|revert)$/;
/** An undo word as the first argument after the script ("bash /x.sh rollback"). */
const UNDO_WORD = /^(?:rollback|undo|revert)$/;
/** A role flag given a value ("--check=false"), which may override the exact flag, so the command's role cannot be told. */
const VALUED_ROLE_FLAG = /^--(?:check|dry-run|rollback|undo|revert)=/;
/** The script of a check or an undo: a path to a shell or Python script file. Any other program (an interpreter, a launcher) may run its arguments. */
const SCRIPT_PATH = /^[\w.~-]*\/[\w./~-]*\.(?:sh|bash|py)$/;
/**
 * The role one simple command has by its own text, or null when it is a run or cannot be told. An allowlist: "sudo" (with only -u USER, -g GROUP,
 * -E, -H, -n or -k) or "runuser -u USER --",
 * then a plain "bash", "sh" or "python3", then the script (a path ending in .sh, .bash or .py), then its arguments. The flag must stand
 * as its own argument after the script or after an argument that is not a flag (a flag before it may take it as its value), with no "--" before it.
 */
function roleOf(command: string): "check" | "undo" | null {
  const t = command.split(/[ \t]+/);
  let i = 0;
  for (;;) {
    if (t[i] === "sudo") {
      i += 1;
      for (;;) {
        if ((t[i] === "-u" || t[i] === "-g") && /^[a-z_][\w-]*$/.test(t[i + 1] ?? "")) i += 2;
        else if (/^-[EHnk]$/.test(t[i] ?? "")) i += 1;
        else break;
      }
    } else if (t[i] === "runuser" && t[i + 1] === "-u" && /^[a-z_][\w-]*$/.test(t[i + 2] ?? "") && t[i + 3] === "--") i += 4;
    else break;
  }
  if (t[i] === "bash" || t[i] === "sh" || t[i] === "python3") i += 1;
  const script = t[i];
  if (script === undefined || !SCRIPT_PATH.test(script) || script.startsWith("-")) return null;
  const args = t.slice(i + 1);
  if (args.some((a) => VALUED_ROLE_FLAG.test(a))) return null;
  // An undo word as the first argument makes the command an undo, but a flag after it can still conflict with it.
  const undoWord = args[0] !== undefined && UNDO_WORD.test(args[0]);
  let role: "check" | "undo" | null = undoWord ? "undo" : null;
  for (let k = undoWord ? 1 : 0; k < args.length; k++) {
    const a = args[k]!;
    if (a === "--") return null;
    const r = CHECK_FLAG.test(a) ? "check" : UNDO_FLAG.test(a) ? "undo" : null;
    if (r === null) continue;
    const before = k === 0 ? script : args[k - 1]!;
    if (k > 0 && before.startsWith("-") && !before.includes("=")) return null;
    if (role !== null && role !== r) return null;
    role = r;
  }
  return role;
}
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

/** A period ends the sentence when a capital word follows. A lowercase word counts only when no flag, quote or escape follows on the line, and not after an abbreviation ("e.g.", "i.e.", "etc."), so "--target=prod. check --dry-run" stays one command and loses no safety flag. */
function sentenceEndsAfter(tail: string): boolean {
  if (/^\s+(?:[A-Z]|$)/.test(tail)) return true;
  return /^\s+[^\s\-&|;<>(]/.test(tail) && !/\s["']?-|["'\\]/.test(tail);
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
    else if (c === "." && i > 0 && /[^\s.]/.test(rest[i - 1]!) && (i + 1 === rest.length || (sentenceEndsAfter(rest.slice(i + 1)) && !/(?:^|\s)(?:e\.g|i\.e|etc|vs)$/i.test(rest.slice(0, i))))) return i;
  }
  return rest.length;
}

/** "scp host:/p /tmp/ then bash /tmp/x.sh": "then" between two commands reads as two steps, so the copied blocks are one command each. */
const THEN_COMMAND = new RegExp(`\\s+then\\s+(?=(?:${COMMAND_WORDS.join("|")})\\s)`);

/** A command has something that only a command has (a path, flag, operator or variable); "bash scripts" alone is a phrase.
 * A flag, operator, "=", variable or host path anywhere counts; a plain file or path counts only as the first argument ("bash x.sh"), so "ssh keys in /srv/k.pub" stays prose. */
function hasCommandPart(piece: string): boolean {
  const args = piece.replace(/^\S+\s+/, "");
  return /[=|&;$]|\w:\/|(?:^|\s)-/.test(args) || /^(?:\S*\/\S*|\S*\w\.\w\S*)(?:\s|$)/.test(args);
}

/** One pair of single backticks ("`bash /x.sh --check`"); a ``double`` span is left alone. */
const BACKTICK_SPAN = /(?<!`)`([^`\n]+)`(?!`)/g;
/** What may stand before a backticked command on its own line: a list number or bullet, which is not part of the command. */
const MARKER_ONLY = /^\s*(?:\(\d{1,3}\)|\d{1,3}[.)]|[-*•])?\s*$/;

/** A command wrapped in one pair of backticks, alone on a line or inside a sentence ("check first: `bash /x.sh --check`").
 * The backticks go and the text inside is the command, character for character (a leading "$ " prompt is dropped); a span
 * with space just inside a backtick, or one the line reader would trim or cut, is left alone. A backticked word that is not
 * a command ("`--check`", "`x.sh`") stays prose. */
function backtickedOf(line: string): { before: string; command: string; after: string } | null {
  for (const m of line.matchAll(BACKTICK_SPAN)) {
    const inner = m[1]!;
    const typed = inner.startsWith("$ ") ? inner.slice(2) : inner;
    if (typed !== typed.trim() || commandOf(inner) !== typed) continue;
    if (inner === typed && !hasCommandPart(typed)) continue;
    const before = line.slice(0, m.index!).replace(/\s+$/, "");
    return { before: MARKER_ONLY.test(before) ? "" : before, command: typed, after: line.slice(m.index! + m[0].length).replace(/^\s+/, "") };
  }
  return null;
}

/** A fence line ("```", "```bash", "~~~", "```text title=x"), with its marker and its whole info string. A backtick fence's info string holds no backtick. */
const FENCE = /^\s*(?:(`{3,})([^`]*)|(~{3,})(.*))$/;
/** Fences whose info string is anything but a plain shell name ("```text", "```json", "```bash title=x") hold samples, not commands to run. */
const SHELL_FENCE = new Set(["", "bash", "sh", "shell", "zsh", "console", "terminal"]);

/** The fence that opens at line i and where it closes (the last line, when it never closes), or null when line i opens none. */
function fenceAt(lines: string[], i: number): { close: number; info: string; inner: string[] } | null {
  const open = FENCE.exec(lines[i]!);
  if (open === null) return null;
  const mark = open[1] ?? open[3]!;
  const info = (open[2] ?? open[4]!).trim().toLowerCase();
  for (let j = i + 1; j < lines.length; j++) {
    const t = lines[j]!.trim();
    if (t.length >= mark.length && t === mark[0]!.repeat(t.length)) return { close: j, info, inner: lines.slice(i + 1, j) };
  }
  // A fence that never closes runs to the end of the text, as in Markdown; it is read as a sample, never as commands.
  return { close: lines.length - 1, info: "unclosed", inner: lines.slice(i + 1) };
}

/** True when a command plainly closes on its own line: its quotes close, it holds no grouping or substitution ("(", "{",
 * a backtick) and no heredoc ("<<"), and it does not end in an operator. Anything subtler keeps the fence one command. */
function selfContained(text: string): boolean {
  return !/[(){}`]|<</.test(text) && quotesClosed(text) && !ENDS_IN_OPERATOR.test(text);
}

/** The commands in a shell fence. When every line is a command that closes on its own and is written with nothing to trim
 * (a "$ " prompt is dropped; a backslash continuation joins the next lines as they are), each is its own block. Otherwise
 * everything between the fence lines is one command exactly as written, if it opens with a command and carries no prompt; failing that, null,
 * and the fence stays as it is. */
function fencedCommands(fence: { info: string; inner: string[] }): string[] | null {
  if (!SHELL_FENCE.has(fence.info)) return null;
  const body = fence.inner;
  if (body.every((l) => l.trim() === "")) return null;
  const each: string[] = [];
  // Split into one block per line only when no character between the fence lines would be left out: a blank line or a carriage return keeps the fence whole.
  let split = body.every((l) => l.trim() !== "" && !l.includes("\r"));
  for (let k = 0; k < body.length && split; k++) {
    const line = body[k]!;
    const typed = line.startsWith("$ ") ? line.slice(2) : line;
    const joined = [typed];
    while (CONTINUES.test(joined[joined.length - 1]!) && k + 1 < body.length) joined.push(body[++k]!);
    const text = joined.join("\n");
    // Nothing trimmed: the line reader must return the first line as written, and the joined text must close on its own.
    split = commandOf(line) === typed && typed === typed.trim() && /\S$/.test(text) && !CONTINUES.test(text) && selfContained(text);
    if (split) each.push(text);
  }
  if (split) return each;
  const first = body.find((l) => l.trim() !== "")!;
  if (body.some((l) => /^\s*\$ /.test(l)) || commandOf(first.replace(/\r$/, "")) === null) return null;
  return [fence.inner.join("\n")]; // every character between the fence lines, blank edge lines included
}

/** Text between two backticked commands on one line that only joins them ("`a` and `b`", "`a`, `b`", "`a` && `b`"): the second shares the first's lead. */
const JOINER = /^[\s,;]*(?:and|or|&&|\|\||&|;)?[\s,;]*$/i;

/** A command written inside a sentence after a colon ("paste one line: scp host:/p /tmp/x && bash /tmp/x (sha256 ...). It builds"). */
function inlineOf(line: string): { before: string; commands: string[]; after: string } | null {
  for (const m of line.matchAll(/:\s+(?=(\S+)\s+\S)/g)) {
    if (!COMMAND_WORDS.includes(m[1]!)) continue;
    const start = m.index! + m[0].length;
    const rest = line.slice(start);
    const end = inlineEnd(rest);
    const whole = trimLine(rest.slice(0, end));
    // "then" is a word, not a step break, inside a quote, after a comment mark or after an escape: the shell reads those differently.
    const pieces = /["'\\]|(?:^|\s)#/.test(whole) ? [whole] : whole.split(THEN_COMMAND);
    if (pieces.some((piece) => commandOf(piece) !== piece)) continue;
    const command = pieces[0]!;
    if (!pieces.every(hasCommandPart)) continue;
    // The whole sentence must not read as prose either ("Note: bash scripts (including this one) need review."): a note in parentheses is not part of the command but is part of that judgement.
    // A digest note ("(sha256 must be <hex>)") is not part of the command and not judged as a sentence, though its words ("must be") are prose.
    // It has to name a hash and hold hex, and the sentence has to stop right after its closing parenthesis. Any other note is part of the sentence.
    const note = rest[end] === "(" ? /^\((?=[^()]*\bsha-?(?:1|256|512)\b)(?=[^()]*[0-9a-f]{8})[^()]*\)(?=\s*(?:\.|$))/i.exec(rest.slice(end)) : null;
    const from = command.length === 0 ? 0 : rest.indexOf(" ") + 1;
    const judged = note !== null ? rest.slice(from, end) : rest.slice(from, inlineEnd(rest, false));
    if (pieces.length > 1 ? pieces.some((piece) => readsAsSentence(piece.slice(piece.indexOf(" ") + 1))) : readsAsSentence(judged)) continue;
    return { before: line.slice(0, start).replace(/\s+$/, ""), commands: pieces, after: rest.slice(end).replace(/^\s+/, "") };
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
  let carried: string | null = null; // the lead of a backticked command whose line is being read again
  for (let i = 0; i < lines.length; i++) {
    const lineLead: string | null = carried;
    carried = null;
    const fence = fenceAt(lines, i);
    if (fence !== null) {
      const fenced = fencedCommands(fence);
      if (fenced === null) {
        prose.push(...lines.slice(i, fence.close + 1)); // the whole fence, both delimiters and all, stays prose
      } else {
        const lead = leadLine();
        flush();
        for (const text of fenced) {
          const part: TextPart = { type: "command", text, role: "step" };
          leads.set(part, lead);
          parts.push(part);
        }
      }
      i = fence.close;
      continue;
    }
    const listed = listedCommandOf(lines[i]!);
    const c = listed === null ? null : listed.command;
    if (c === null) {
      const ticked = backtickedOf(lines[i]!);
      if (ticked !== null) {
        const joined: boolean = lineLead !== null && JOINER.test(ticked.before);
        if (ticked.before !== "") prose.push(ticked.before);
        const lead: string = joined && lineLead !== null ? lineLead : leadLine();
        flush();
        const part: TextPart = { type: "command", text: ticked.command, role: "step" };
        leads.set(part, lead);
        parts.push(part);
        // The rest of the line is read again: it may hold another backticked command.
        if (ticked.after !== "") {
          lines[i--] = ticked.after;
          carried = lead;
        }
        continue;
      }
      const inline = inlineOf(lines[i]!);
      if (inline === null) {
        prose.push(lines[i]!);
        continue;
      }
      prose.push(inline.before);
      const lead = leadLine();
      flush();
      for (const text of inline.commands) {
        const part: TextPart = { type: "command", text, role: "step" };
        leads.set(part, lead);
        parts.push(part);
      }
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
  // A check or an undo by its text is made only of simple commands that each are one (roleOf): commands joined by a line break, &&, || or ; are judged
  // one by one. Anything else may run a command the flag does not reach, so it makes the block a run: a substitution anywhere (even in quotes), a pipe,
  // a background &, a comment, a backslash (a backslash-newline is removed, as the shell removes it, so "--check\\" then "=false" reads "--check=false"), an unclosed quote, a "$", glob or brace that the shell would expand, or whitespace other than a space,
  // a tab or a line break (the shell keeps a no-break space or a carriage return inside the argument). A flag in quotes is read as the shell passes
  // it, so it counts for its conflicts, and a redirection that stands as its own word, with its file name ("2>&1", "> /tmp/log", but also "> --check"),
  // is dropped whole, quoted parts and all, before judging; one glued to an argument ("--check2>/tmp/log") is left, so the command is a run.
  const every = (text: string, role: "check" | "undo"): boolean => {
    if (/[$`]|[<>]\(|[^\S \t\n]/.test(text)) return false;
    // Quotes go as the shell removes them, so a quoted flag ("--check=false", --dry"-run") is the word the script gets; any character in quotes that
    // could not be part of a flag becomes "_", which no flag holds and nothing below reads as an operator.
    const bare = text.replace(/\\\n/g, "").replace(/"((?:\\.|[^"\\])*)"|'([^']*)'/g, (_m, d?: string, q?: string) => (d ?? q ?? "").replace(/[^\w=.,/:@%+~-]/g, "_"));
    if (/["'\\*?[\]{}]/.test(bare)) return false;
    const unredirected = bare.replace(/(?<=^|\s)\d*>&\d+(?=\s|$)/gm, " ").replace(/(?<=^|\s)(?:&>>?|\d*>>?|\d*<)\s*[^\s;&|<>]+/gm, " ");
    const commands = unredirected.split(/\n|&&|\|\||;/).map((l) => l.trim()).filter((l) => l !== "");
    return commands.length > 0 && commands.every((l) => !/[|&#<>]/.test(l) && roleOf(l) === role);
  };
  const kind = (c: Extract<TextPart, { type: "command" }>): CommandRole =>
    UNDO_LEAD.test(leads.get(c) ?? "") || every(c.text, "undo") ? "undo" : every(c.text, "check") ? "check" : "step";
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
