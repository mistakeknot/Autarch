# Agent workbench interaction pass

Date: 2026-09-13

Scope: one bounded pass to resolve how Autarch should present continuing work,
attention, and pane navigation. Product pages below describe their own behavior;
the Autarch column records behavior observed in this checkout before the MVP.

| Pattern | Documented elsewhere | Observed in Autarch before this slice | MVP decision |
|---|---|---|---|
| Continue a project | Cursor Projects keeps project context over time, coordinates agents, and retains shared files and learned conventions. | The project HUD reads product sources and Beads, but it does not retain a chosen outcome, task, pane, or unsent direction. | Reopen directly into Outcome / Current work / Agent / Next action, restoring local choices without sending anything. |
| Direct attention | Linear Priority Inbox separates priority from other items; its project drafting and agent activity surfaces keep edits and activity compact. | Current work is one section among source-heavy details, and agent questions live on separate screens. | Put the next decision first, keep full sources and receipts behind details, and label agent report, verification, and human verdict separately. |
| Navigate live agents | cmux documents persistent workspace tabs, splits, notification rings, cwd/branch labels, and CLI/socket control for sending, reading, and opening panes. | Autarch inventories only each tmux session's active pane and switches by session display name. | Inventory every pane, address exact socket/server/session/window/pane identity, show bounded local output, and never treat names as authority. |

Sources:

- Cursor, “Projects: Long-running agents and a new way to collaborate with agents” — <https://cursor.com/blog/projects>
- Linear changelog, “Priority Inbox and drafting projects with agents” — <https://linear.app/changelog/2026-09-03-priority-inbox>
- cmux product page — <https://cmux.com/>

This pass is complete. Further competitor research should begin only with a
specific unresolved interaction from the live Autarch walkthrough.
