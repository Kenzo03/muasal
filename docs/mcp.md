# MCP for AI agents

## What it is

AI agents such as Claude Code can work with your tickets through the Model Context Protocol (MCP). The endpoint is `/mcp`. You approve each agent once in the browser. After that it keeps its own token.

An agent sees only what you can see, and the same permissions, validation and audit apply as in the app.

## Connect Claude Code

```bash
claude mcp add --transport http muasal https://muasal.example.com/mcp
```

Use your own address in place of `muasal.example.com`. Then run `/mcp` in Claude Code, pick **muasal**, and sign in. A Muasal page asks you to approve the agent.

## Other clients

Any MCP client that supports streamable HTTP and OAuth sign-in works. The URL is `<PUBLIC_URL>/mcp`. Two limits apply today:

- Redirect URIs must be `https`, or `http` on localhost. Clients that use custom URL schemes cannot register yet.
- Browser-based clients are not supported, because Muasal sends no CORS headers.

**Claude Desktop and claude.ai.** Add Muasal as a custom connector with the `/mcp` URL. claude.ai connects from Anthropic's servers, so Muasal must be reachable at a public HTTPS `PUBLIC_URL`.

Every tool call counts toward the token's 60 requests a minute, and `get_project` makes several API calls.

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
| `create_node` | Adds a module or menu to the project's tree. Project admins only. |
| `update_node` | Renames, moves, archives or restores a module or menu, or changes its code, aliases or clients. Project admins only. |
| `import_tree` | Imports a module-tree CSV. It previews unless you pass `apply`. Project admins only. |

`update_ticket` changes only the fields you pass. It cannot clear a field: to unassign, remove a due date, or move a ticket back to core work, use the web app.

The tree tools follow the same rules as the web app: a member who is not a project admin gets an error. There is no tool to delete a node; archive it with `update_node` instead.

## Read-only access

Tick **Read only** on the approval page. The agent can then list and read tickets, and every write tool returns an error.

## Disconnect an agent

Open **Settings › API tokens** and revoke the token named after the agent, with "(MCP)" after it. The agent loses access at once. Signing in again creates a new token, so revoke old ones you no longer use.

## Why there is no delete

Muasal keeps why every change happened. `cancel_ticket` closes a ticket as Cancelled with its decision record, so the history stays.

## Behind your own proxy

Route `/mcp`, `/oauth/register`, `/oauth/token` and `/.well-known/oauth-*` to the app, like `/api`. `/oauth/authorize` is a web page and goes to the web app.
