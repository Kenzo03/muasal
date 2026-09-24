# Muasal

Know why every screen is the way it is. Muasal is a self-hosted ticketing tool that remembers the reason behind every change to every menu, and answers "why does this screen work like this?" with links to the tickets behind it.

Status: early development (Iteration 0: sign-in and user management).

## Run it

    cp deploy/.env.example deploy/.env      # then change both passwords
    make up                                  # builds and starts Caddy, web, app and PostgreSQL
    make admin EMAIL=you@example.com NAME="Your Name"

Open the printed setup link, choose a password, and sign in at http://localhost.

## Develop

    make testdb      # throwaway PostgreSQL on localhost:55432 for the Go tests
    make test        # Go unit and integration tests
    make generate    # regenerate Go stubs, sqlc code and TypeScript API types
    make e2e         # browser test against a running `make up` stack

## License

Apache-2.0. See LICENSE and NOTICE.
