# Muasal

**Know why every screen is the way it is.**

Muasal is a self-hosted ticketing tool for teams that build and maintain software for several clients. Every ticket is linked to the menus it changes, and closing it records what changed and why. A year later, anyone can open a menu and read its history, or ask "why does overtime approval skip the supervisor for Client A?" and get an answer with a citation on every claim.

- **Module tree:** the product's modules and menus, with the clients each one serves.
- **Tickets with decision records:** a ticket cannot close without its reason and menus; closing asks what changed, why and what was rejected.
- **Menu pages:** each menu's history newest first, and the decisions in force for each client.
- **Ask:** answers from your tickets, decision notes and documents, from a local model, your own API key, or with AI off as keyword search. It never uses a ticket the asker cannot open.
- **Change summaries:** what changed on a module for a client, printable as PDF.
- **Imports:** tickets from Jira or CSV, the tree from CSV, and the tree drafted from your FSD.

Muasal runs on your own server with Docker Compose. In local or off AI mode it makes no outbound calls. The interface is in Indonesian and English.

## Where to start

- [Hardware guide](hardware.md): pick a server.
- [Running Muasal](operations.md): get a release, install, upgrade, back up.
- [User guide](user-guide.md): the everyday work of PMs, developers and admins.
- [Pilot kit](pilot.md): run a one-month pilot with one team.
- [Security review](security/asvs-l1.md): OWASP ASVS level 1.

Muasal is open source under the Apache-2.0 licence. To report a vulnerability, see the [security policy](https://github.com/Kenzo03/muasal/security/policy); to contribute, see [CONTRIBUTING.md](https://github.com/Kenzo03/muasal/blob/main/CONTRIBUTING.md).
