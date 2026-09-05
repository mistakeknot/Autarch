import fs from "node:fs/promises";
import path from "node:path";

const string = { type: "string" };
const strings = { type: "array", items: string };
const object = (properties, required = Object.keys(properties)) => ({ type: "object", properties, required, additionalProperties: false });

export default function investigator(pi) {
    const root = process.env.AUTARCH_REVIEW_PROJECT;
    pi.registerTool({
        name: "read_project", label: "Read project evidence",
        description: "Read a bounded project file or list a project directory. Read-only; paths are confined to this project. Cite the returned path and revision.",
        parameters: object({ path: string }),
        async execute(_id, params) {
            const target = await fs.realpath(path.resolve(root, params.path));
            const relative = path.relative(await fs.realpath(root), target);
            if (relative.startsWith("..") || path.isAbsolute(relative) || relative.split(path.sep).some(part => part === ".git" || part === "node_modules" || part.startsWith(".env"))) throw new Error("Path outside review evidence scope");
            const stat = await fs.stat(target);
            if (stat.isDirectory()) return { content: [{ type: "text", text: JSON.stringify((await fs.readdir(target)).slice(0, 200)) }], details: { path: target } };
            if (!stat.isFile() || stat.size > 256 * 1024) throw new Error("Evidence file is unavailable or exceeds 256 KiB; choose a smaller source");
            const content = await fs.readFile(target, "utf8");
            return { content: [{ type: "text", text: `${target}\n${content}` }], details: { path: target, modified: stat.mtime.toISOString() } };
        }
    });
    pi.registerTool({
        name: "ask_review_question", label: "Project decision",
        description: "Ask one consequential question. During testing the question waits in review. After the answer, immediately continue to the next needed question or prepare the response.",
        parameters: object({ question: string, options: strings }),
        async execute(_id, params, _signal, _update, ctx) {
            const answer = params.options.length ? await ctx.ui.select(params.question, params.options, { signal: _signal }) : await ctx.ui.input(params.question, undefined, { signal: _signal });
            return { content: [{ type: "text", text: answer === undefined ? "Cancelled; do not infer an answer." : answer }], details: { answered: answer !== undefined } };
        }
    });
    const guidance = object({ path: string, text: string, scope: string, rationale: string, base_revision: string, supersedes: string }, ["path", "text", "scope", "rationale", "base_revision"]);
    pi.registerTool({
        name: "propose_response", label: "Prepare response for review",
        description: "Propose an immediate change and any enduring guidance together. No implementation or approval happens here. Include observed feedback IDs, evidence, uncertainty, reasoned pushback, exact scope, priority, dependencies, budget and a short retest checklist.",
        parameters: object({ outcome: string, change: string, scope: strings, rationale: string, feedback_ids: strings, uncertainties: strings, pushback: string, guidance: { type: "array", items: guidance }, checklist: strings, priority: { type: "integer", minimum: 0, maximum: 4 }, dependencies: strings, budget_tokens: { type: "integer", minimum: 1, maximum: 500000 } }),
        async execute(_id, params) { return { content: [{ type: "text", text: "Response prepared for human review. No change has been accepted or implemented." }], details: { autarchProposal: params } }; }
    });
}
