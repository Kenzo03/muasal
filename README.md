# Muasal

Know why every screen is the way it is. Muasal is a self-hosted ticketing tool that remembers the reason behind every change to every menu, and answers "why does this screen work like this?" with links to the tickets behind it.

Status: MVP, getting ready for the first pilot (FSD §21 Iteration 6). It covers the module tree, tickets with decision records, node history, search, and Ask: answers with a citation on every claim from a local model, your own key, or AI off.

## Run it

    cp deploy/.env.example deploy/.env      # then change both passwords
    make up                                  # builds and starts Caddy, web, app and PostgreSQL
    make admin EMAIL=you@example.com NAME="Your Name"

Open the printed setup link, choose a password, and sign in at http://localhost.

To install on a server, from the offline bundle or online, see [docs/operations.md](docs/operations.md). To choose hardware, see [docs/hardware.md](docs/hardware.md).

## Develop

    make testdb      # throwaway PostgreSQL on localhost:55432 for the Go tests
    make test        # Go unit and integration tests
    make generate    # regenerate Go stubs, sqlc code and TypeScript API types
    make e2e         # browser test against a running `make up` stack
    deploy/loadtest/run.sh 100000   # the 100,000-ticket load test against the stack (FSD §21.1)

## License

Apache-2.0. See LICENSE and NOTICE.
