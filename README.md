# Muasal

Know why every screen is the way it is. Muasal is a self-hosted ticketing tool that remembers the reason behind every change to every menu, and answers "why does this screen work like this?" with links to the tickets behind it.

It covers the module tree, tickets with decision records, menu history, search, and Ask: answers with a citation on every claim from a local model, your own key, or AI off. On top of that come imports from Jira and CSV, AI-drafted decision records, change summaries, Git webhooks, notifications, and module trees drafted from your specification.

**Documentation:** https://kenzo03.github.io/muasal/

## Install a release

Download a release from [GitHub releases](https://github.com/Kenzo03/muasal/releases). It includes the offline bundle, the deploy files for online installs, and signed checksums; its images are signed and built for amd64 and arm64. Then follow [docs/operations.md](docs/operations.md#get-a-release).

## Run it from source

    cp deploy/.env.example deploy/.env      # then change both passwords
    make up                                  # builds and starts Caddy, web, app and PostgreSQL
    make admin EMAIL=you@example.com NAME="Your Name"

Open the printed setup link, choose a password, and sign in at http://localhost.

To install on a server, from the offline bundle or online, see [docs/operations.md](docs/operations.md). To choose hardware, see [docs/hardware.md](docs/hardware.md); to run a pilot, [docs/pilot.md](docs/pilot.md).

## Develop

    make testdb      # throwaway PostgreSQL on localhost:55432 for the Go tests
    make test        # Go unit and integration tests
    make generate    # regenerate Go stubs, sqlc code and TypeScript API types
    make e2e         # browser test against a running `make up` stack
    deploy/loadtest/run.sh 100000   # the 100,000-ticket load test against the stack (FSD §21.1)

## Contribute

See [CONTRIBUTING.md](CONTRIBUTING.md). Report vulnerabilities privately, as [SECURITY.md](SECURITY.md) explains.

## License

Apache-2.0. See LICENSE and NOTICE.
