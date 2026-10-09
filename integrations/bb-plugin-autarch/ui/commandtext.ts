// A card filed as free text carries its commands as prose. This splits a question into prose and command lines so
// each command can be one copyable block (bead mk-yjp7). Pure: nothing is run, and a command's text is returned
// exactly as written (only a leading "$ " prompt is dropped, because it is not part of the command).
export type CommandRole = "check" | "step" | "final";
export type TextPart = { type: "prose"; text: string } | { type: "command"; text: string; role: CommandRole };

const COMMAND_WORDS = ["bash", "sudo", "runuser", "scp", "ssh", "sh", "python3", "gh", "bb", "bd", "autarch", "sonnerie"];
const DRY_RUN = /(^|\s)--(check|dry-run)(\s|=|$)/;

const SENTENCE_START = /^(is|are|was|were|the|a|an|it|that|this|to|and|or|if|when|then)\b/i;

/** The last word reads as a file or path ("x.sh", "/tmp/o", "--out=/x"), so a closing period belongs to it. */
const PATH_LIKE = /[/\\=]|\w\.\w/;

// Words that make a line a sentence. Words that are also real arguments ("all", "list", "for", "with", "only") are left out.
const PROSE_WORDS = new Set(("the a an is are was were be been to of that this these those it its because when then if will can could should would may might has have had does supports support reports shows prints returns outputs works means gives lets says uses helps").split(" "));

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
  const lines = text.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const c = commandOf(lines[i]!);
    if (c === null) {
      prose.push(lines[i]!);
      continue;
    }
    flush();
    const whole = [c];
    while (CONTINUES.test(whole[whole.length - 1]!) && i + 1 < lines.length) whole.push(trimLine(lines[++i]!, true));
    parts.push({ type: "command", text: whole.join("\n"), role: "step" });
  }
  flush();
  const cmds = parts.filter((p): p is Extract<TextPart, { type: "command" }> => p.type === "command");
  cmds.forEach((c, i) => {
    c.role = i === cmds.length - 1 ? "final" : DRY_RUN.test(c.text) ? "check" : "step";
  });
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
