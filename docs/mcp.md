# MCP for AI agents

## What it is

AI agents such as Claude Code can work with your tickets through the Model Context Protocol (MCP). The endpoint is `/mcp`. You approve each agent once in the browser. After that it keeps its own token.

An agent sees only what you can see, and the same permissions, validation and audit apply as in the app.

## Connect Claude Code

```
claude mcp add --transport http muasal https://muasal.example.com/mcp
```

Use your own address in place of `muasal.example.com`. Then run `/mcp` in Claude Code, pick **muasal**, and sign in. A Muasal page asks you to approve the agent.

## Other clients

Any MCP client that supports streamable HTTP and OAuth sign-in works. The URL is `<PUBLIC_URL>/mcp`.

## Tools

| Tool | What it does |
| --- | --- |
| `get_project` | Reads a project's statuses, menu tree, clients and assignees. Call it first for the ids the other tools need. |
| `list_tickets` | Lists the tickets you may see, with filters, sorting and paging. |
| `get_ticket` | Reads one ticket by key, such as `HRIS-12`, with its decision record and version. |
| `create_ticket` | Files a ticket. Send the same `idempotency_key` when you retry. |
| `update_ticket` | Changes the fields you pass. Pass `version` to fail instead of overwriting a newer change. |
| `transition_ticket` | Moves a ticket to another status. Closing needs a reason, a menu and a decision. |
| `cancel_ticket` | Closes a ticket as Cancelled and records why. |

`update_ticket` changes only the fields you pass. It cannot clear a field: to unassign, remove a due date, or move a ticket back to core work, use the web app.

## Read-only access

Tick **Read only** on the approval page. The agent can then list and read tickets, and every write tool returns an error.

## Disconnect an agent

Open **Settings › API tokens** and revoke the token named after the agent, with "(MCP)" after it. The agent loses access at once.

## Why there is no delete

Muasal keeps why every change happened. `cancel_ticket` closes a ticket as Cancelled with its decision record, so the history stays.

## Behind your own proxy

Route `/mcp`, `/oauth/register`, `/oauth/token` and `/.well-known/oauth-*` to the app, like `/api`. `/oauth/authorize` is a web page and goes to the web app.
