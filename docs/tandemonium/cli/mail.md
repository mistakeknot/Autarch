# Mail CLI

The mail CLI mirrors MCP Agent Mail workflows for local coordination and testing.

## Commands

Send a message:

```bash
tandemonium mail send \
  --to bob \
  --from alice \
  --subject "Hello" \
  --body "Body" \
  --thread thread-1 \
  --id msg-001 \
  --importance normal \
  --ack
```

Reply:

```bash
tandemonium mail reply --id msg-001 --from bob --body "Reply body"
```

Attachments:

```bash
tandemonium mail send \
  --to bob \
  --subject "Spec" \
  --body "See attachment" \
  --attach /path/to/spec.md::notes
```

Inbox and search:

```bash
tandemonium mail inbox --recipient bob --json

tandemonium mail search --query "auth" --limit 20 --json
```

Acknowledge and mark read:

```bash
tandemonium mail ack --recipient bob --id msg-001

tandemonium mail read --recipient bob --id msg-001
```

Summaries:

```bash
tandemonium mail summarize --thread thread-1

tandemonium mail summarize --thread thread-1 --llm --examples --json

tandemonium mail summarize --dry-run --json
```

Contact policies and requests:

```bash
tandemonium mail policy set --owner bob --policy contacts_only

tandemonium mail policy get --owner bob

tandemonium mail contact request --requester alice --recipient bob

tandemonium mail contact respond --requester alice --recipient bob --accept
```

## LLM summary configuration

Add to `.tandemonium/config.toml`:

```toml
[llm_summary]
command = "claude"
timeout_seconds = 30
```

Notes:
- `--llm` or `--examples` invokes the configured command to summarize thread messages.
- `--dry-run` sends synthetic messages to validate the command wiring without a real thread.
- The command must emit JSON with `summary` and optional `examples` fields.
- `mail reply` defaults to `Re:` subject prefix; override with `--subject` or `--subject-prefix`.

## Output formats

Most commands accept `--json` for machine-readable output.

### JSON examples

Send:

```json
{
  "id": "msg-001",
  "thread_id": "thread-1",
  "recipients": ["bob"]
}
```

Inbox:

```json
{
  "messages": [
    {
      "id": "msg-001",
      "thread_id": "thread-1",
      "sender": "alice",
      "subject": "Hello",
      "created_ts": "2026-01-21T12:34:56Z",
      "recipient": "bob"
    }
  ],
  "next_token": ""
}
```

Summarize (LLM):

```json
{
  "thread_id": "thread-1",
  "participants": ["alice", "bob"],
  "message_count": 2,
  "key_points": ["Summary point"],
  "action_items": ["Follow up"],
  "examples": [
    {"id": "msg-001", "subject": "Hello", "body": "Body"}
  ]
}
```
