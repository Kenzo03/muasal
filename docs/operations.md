# Running Muasal

This guide installs, upgrades, backs up and restores Muasal on one Linux server (FSD §19). It ships as the README of the offline bundle, `muasal-<version>.tar`.

## What you need

- Linux (x86-64 or ARM64) with Docker Engine and Docker Compose v2.
- For the recommended tier, an NVIDIA GPU with its driver and the NVIDIA Container Toolkit.
- Ports 80 and 443 free, and a DNS name or IP address for the server.
- For HTTPS, a certificate and key (PEM), or Caddy's internal CA.

Hardware tiers (FSD §18.1):

| Tier | Hardware | Chat model | Expected Ask median |
| --- | --- | --- | --- |
| Minimum | 8+ CPU cores, 32 GB RAM, SSD, no GPU | `qwen3.5:4b` | 45–120 s (estimate) |
| Recommended | GPU with 12–16 GB VRAM, 32 GB RAM | `qwen3.5:9b` | 15 s or less (target) |

Embeddings use `bge-m3` on every tier. AI is optional: with AI off, Ask answers with keyword results and nothing needs a model.

## Get a release

Each release on the [GitHub releases page](https://github.com/Kenzo03/muasal/releases) has:

| File | Use it for |
|---|---|
| `muasal-<version>.tar` | The offline bundle: every image the stack runs (linux/amd64) and the deploy files. |
| `muasal-deploy-<version>.tar.gz` | The deploy files alone, for `./install.sh --online` on amd64 or arm64. |
| `SHA256SUMS`, `SHA256SUMS.sigstore.json` | Checksums of both, signed by the release workflow. |

The app and web images are also on GHCR as `ghcr.io/kenzo03/muasal-app` and `ghcr.io/kenzo03/muasal-web`, for linux/amd64 and linux/arm64, with an SBOM and build provenance.

**Verify what you downloaded** with [cosign](https://docs.sigstore.dev/cosign/system_config/installation/):

```sh
cosign verify-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/[Kk]enzo03/muasal/\.github/workflows/release\.yml@refs/tags/v'
sha256sum -c SHA256SUMS --ignore-missing
```

The first command proves that the checksums come from this repository's release workflow; the second, that your files match them. With cosign installed, `install.sh --online` also verifies both images' signatures before it starts anything.

**ARM servers** (arm64) install online: unpack `muasal-deploy-<version>.tar.gz` and run `./install.sh --online`. The offline bundle is amd64 only.

## Install

```sh
tar -xf muasal-<version>.tar
cd muasal-<version>
./install.sh
```

The installer asks for the public URL, the AI mode (off, local or byok), the hardware tier and the first admin. It then does the rest:
- loads the images and writes `.env` with new database passwords and an `APP_SECRET_KEY`;
- copies the model files into the models volume (local AI only);
- starts the stack and creates the first admin, printing their one-time setup link;
- in local mode, sets Admin → AI to the tier's presets.

To install without questions:

```sh
./install.sh --yes --url https://muasal.example.co.id --cert /path/cert.pem --key /path/key.pem \
  --ai local --tier recommended --admin-email it@example.co.id --admin-name "IT Admin"
```

`--online` pulls the signed images from GHCR and the models from Ollama instead of the bundle; run it from the release's deploy files. After the install, the server makes no outbound calls in local or off mode.

**First use:** open the setup link, set a password, then create a project, build the module tree and add members.

**BYOK:** choose `--ai byok`, then enter the provider's URL and key in Admin → AI and confirm the acknowledgement. Questions and ticket excerpts then go to that provider.

## Upgrade and roll back

```sh
./upgrade.sh muasal-<new version>.tar
```

The script:
1. takes a backup and waits for it to finish;
2. loads the new images and copies in the new deploy files, keeping `.env`, certificates and the TLS setting;
3. restarts the stack. The app applies forward-only migrations as it starts.

Release notes say when a re-index is needed; the app queues it itself.

**Roll back:** restore the pre-upgrade backup with `./restore.sh`, set `VERSION` in `.env` back to the old version, and run `docker compose --env-file .env up -d`.

## Backups and restore

The `backup` service dumps the database every day at 01:00 server time (`TZ` in `.env`) into the `backups` volume. It also copies new attachments there and keeps 14 days of dumps.

The dumps leave out the embedding vectors, since they can be rebuilt. Set `BACKUP_INCLUDE_VECTORS=true` in `.env` to keep them, for example when a CPU-tier rebuild would take too long.

Admin → Backups shows the last backup's time, size and location, and has a "Run backup now" button; the backup starts within 30 seconds.

Copying the backups volume off the server is up to you. For example, mount a NAS share at the volume's path, or copy `docker run --rm -v muasal_backups:/b alpine tar -C /b -c .` to tape.

**Restore:**

```sh
./restore.sh db-20260926-0100.dump
```

The script stops the app and the web server and restores the database with `pg_restore --clean`. It then copies the attachments back, starts both services and queues a re-index. Run a restore drill once a month into a scratch copy of the stack.

## Status and monitoring

- **Admin → System status:** database size, the job queue, the model server's health, and the disk use of the attachments and backups volumes. It warns from 80% full.
- **Logs:** `docker compose logs -f app` (JSON lines). Logs carry IDs, never ticket text or questions.
- **Metrics:** Prometheus metrics at `http://app:8080/metrics` on the internal network. Caddy does not publish them; scrape them from a container on the `app` network.
- **Ask log retention:** 365 days, set by `ASK_LOG_RETENTION_DAYS` in `.env` (0 keeps questions for good).

## Email notifications

Muasal can email people what they have not read in the app, so assignments and mentions reach those who do not keep it open. It is off until an admin sets it up.

1. In `.env`, set `SMTP_HOST` and `SMTP_FROM` (the sender, e.g. `muasal@example.com`), plus `SMTP_USERNAME` and `SMTP_PASSWORD` if the server asks for them. `SMTP_TLS` is `starttls` (the default, port 587), `tls` (port 465) or `none` for a relay on your own network; `SMTP_PORT` overrides the port.
2. Give the app a route to the mail server: run with `-f compose.host-ai.yaml`, or use a relay on the `app` network.
3. Restart: `docker compose up -d app`. Admin → System status shows Email as on.

Each person then ticks **Also email me** in their profile. Every minute, Muasal sends each of them one email listing their notifications of the last day that are still unread after two minutes, each with a link; nothing is sent twice.

## Offline guarantees

- Only Caddy publishes ports.
- The app, web, database, backup and model services sit on an internal network with no route to the internet.
- BYOK is an admin opt-in and opens one route, to its provider. Run `docker compose -f compose.yaml -f compose.host-ai.yaml` for it.
- Next.js telemetry is off.
