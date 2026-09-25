# Muasal Iteration 4a — Indexing, AI Modes and the Ask Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Status: frame for review.** This file holds the goal, the design, the deviations and the task list with each task's interfaces. The steps of each task, with full code and tests, are written once the frame is agreed.

**Goal:** Ask answers from the tickets a user may open, with a citation on every claim, or says it lacks information. This is the exit check for FSD §21 Iteration 4, as agreed on 25 Sep 2026: golden set v0 reaches 90% citation precision with `qwen3.5:4b` on the dev laptop (MacBook Air M4, 16 GB). To get there:
- every saved ticket, comment and decision record is chunked and embedded by a job queued in the same transaction (FSD §13);
- a system admin picks the AI mode (Off, Local or BYOK) and every model setting in Admin → AI, with no restart (§13.4);
- `POST /ask` detects the scope, retrieves evidence under the visibility predicate, streams schema-constrained claims and drops every claim without a valid citation (§10, §11);
- `app eval` runs the golden set and reports the four §11.8 measures.

Iteration 4 is split in two (decided 25 Sep 2026). This plan, 4a, is the AI backend plus the Admin → AI page. Plan 4b holds Iteration 4's web items: the create modal with the `c` shortcut, markdown rendering, module-tree drag and drop, and recently used menus first. The Ask UI (page, panel, node tab, Home Ask box) and the Ask log screens stay in Iteration 5.

**Architecture:** Same stack and patterns as Iterations 0–3.
- **Jobs:** River 0.47 in PostgreSQL (FSD §2 row 3). `app migrate up` runs River's migrator after goose and before the app role's grants. `serve` starts one River client with an `index` queue. `inTx` hands its `pgx.Tx` to River's `InsertTx`, so an index job commits or rolls back with its change (§4.2).
- **`llm` package:** one small OpenAI-compatible client for `/v1/models`, `/v1/chat/completions` (streaming, `response_format` with a JSON schema, `reasoning_effort: "none"`) and `/v1/embeddings`. Local (Ollama or vLLM) and BYOK use the same client with different URLs and keys.
- **AI settings:** one row in `settings` (key `ai`), read per request and cached for 5 seconds, so a change applies to the next question without a restart. API keys are sealed with AES-GCM under `APP_SECRET_KEY` (`internal/secret`) and never returned.
- **`indexer` package:** builds a source's chunks (ticket header, comment, decision record), compares content hashes, embeds only changed chunks in batches of 32, and upserts them under a per-source advisory lock. A second job embeds chunks that still lack a vector: after the model server was down, after Off → Local, or after an embedding-model change.
- **`ask` package:** scope detection without a model call, retrieval in SQL under the visibility predicate (exhaustive for ≤ 40 items, else vector + keyword with reciprocal rank fusion), evidence packing within the token budget, the prompt, a streaming JSON decoder that emits each claim as its object closes, and the validator. The server decides the status.
- **HTTP:** `POST /ask` streams SSE (`queued`, `scope`, `evidence`, `claim`, `result`, `error`) or returns JSON by `Accept`. A semaphore (`ask.max_concurrent`) gates generations and pauses embedding while any runs.
- **Web:** `/admin/ai` for system admins: mode, chat and embedding endpoints and models, the write-only API key, the BYOK acknowledgement, Test connection, and Index status with Retry and Re-index all.
- **Deploy:** an optional `model` service (Ollama) under the `local-ai` compose profile on the internal network, and `deploy/compose.host-ai.yaml` for the laptop, where Ollama runs natively on the Apple GPU.

**Tech Stack:** adds River 0.47 (`riverqueue/river`, `riverpgxv5`, `rivermigrate`) and `pgvector-go` 0.4 to the server. No new web dependencies.

**Spec:** Claude Docs "FSD — Muasal" (rev 108):
- §10: what Ask shows, including not enough information, language and acceptance criteria.
- §11: the Ask engine: grants, scope detection, retrieval, packing, prompt, validation, stream events, concurrency, quality gate.
- §13: indexing, freshness, AI modes and model swap, AC-IX-1 to AC-IX-6.
- §16: `chunks`, `ask_threads`, `ask_queries`, `settings`.
- §17: `POST /ask`, `/ask/threads`, `/admin/settings/ai`, `/admin/ai/*`.
- §18.1: tier presets and the laptop measurements.
- §21: delivery plan.

## Global Constraints

- **Carried over:** everything in the Iteration 0–3 Global Constraints still holds, notably:
  - "hidden and missing look the same" (404);
  - "one transaction per mutation with its audit row", and now its index jobs too;
  - the visibility predicate lives in SQL; no Go loop filters rows.
- **Evidence scope (AC-AK-3):** every retrieval query carries the membership check of Iteration 3's search, on `chunks.project_id` and `chunks.client_id`. Internal comments are evidence for members (they can read them on the ticket).
- **Not enough information (AC-AK-5):** "Not enough information in the tickets you can access to answer this." Never a hint that hidden tickets exist. With no evidence, the chat model is not called, and the log records `llm_called = false`.
- **Validation (AC-AK-6):** a citation outside the evidence is dropped; a claim left without citations is dropped; a claim whose text names a key outside the evidence is dropped. No surviving claim means not enough information, with up to five closest visible tickets.
- **Off mode (AC-IX-5):** `POST /ask` returns keyword results under the same scope, with the banner code `ai_off`, and makes no model call. The indexer still writes chunks without vectors (R-AI-5).
- **BYOK (R-AI-1, AC-IX-6):** saving BYOK without the acknowledgement answers 422 and sends nothing to the provider. The acknowledgement is audited. API keys are never returned or logged (R-AI-3).
- **Freshness (AC-IX-1, AC-IX-2):** a save never fails because the model server is down; its job retries with backoff, up to 10 attempts. A change is retrievable within 30 seconds (p95) under normal load.
- **Timeouts:** 60 seconds per answer; 90 seconds waiting for a model slot, then "AI server busy".
- **Rate limit:** Ask 10 per minute per user (§17.1).
- **Logs:** carry IDs, never ticket text or questions; question text lives only in `ask_queries` (§18.2).
- **UI strings:** every string ships in Indonesian (default) and English.
- **AI is the installer's choice:** Muasal runs fully without a model. Whoever installs it picks, in Admin → AI, either a local model server they run (Ollama or vLLM) or their own cloud key (BYOK); nothing assumes a model is present. Tickets, search, node pages and Home never depend on AI.

## Deliberate Deviations from the FSD

**Scope**
- Iteration 4 is split into 4a (this plan) and 4b (web items), each with its own branch and PR (decided 25 Sep 2026).
- The exit check uses `qwen3.5:4b` on the dev laptop instead of `qwen3.5:9b` (decided 25 Sep 2026). 9b stays the recommended tier's default and is checked on a GPU server later. §21 changes to match.
- The Admin → AI page ships here, because the exit check needs Local mode set without SQL. The Ask log screens (`/admin/ask-log`) come with the Ask UI in Iteration 5; every question is logged from this iteration on.
- Follow-ups (§11.9), feedback (§10.7), decision notes, commits and documents as evidence are P1 and stay out. Threads hold independent questions.

**Golden set v0**
- There is no pilot team yet, so v0 is built on a demo dataset in the repo, `server/eval/demo-hris.json`: one project, three clients, about 60 tickets with decision records and comments, in Indonesian, English and mixed text. `app eval --seed` loads it into a fresh project.
- `server/eval/golden-v0.jsonl` holds 40 answerable questions (at least 40% Indonesian or mixed, covering the why, what-changed and onboarding questions of stories 2, 4 and 6) and 10 unanswerable ones. The FSD's 50 real questions per pilot team replace it at the pilot.

**Engine**
- Scope detection matches people by user and contact names after a cue word, as specified. Date phrases cover the §11.2 list in both languages; other phrasings are left to the explicit date chip.
- Language detection counts common Indonesian and English words, as specified; no model call.
- The relevance floor `ask.min_similarity` starts at 0.45 and is tuned by the exit-check run; the chosen value goes into the tier presets.
- A fresh install starts in **Off** mode instead of Local (§13.4 table), because the model is the installer's choice (25 Sep 2026). The installer or the admin switches to Local or BYOK; chunks written while Off get their vectors afterwards (R-AI-5).
- `/readyz` does not check the model server, so the stack stays healthy while Ollama is down (AC-IX-2). Index status and Test connection report the model server instead.
- An embedding-model change clears vectors, changes the column dimension and rebuilds the HNSW index in one migration-like step run by a job, as §13.4 describes; the shadow-table upgrade stays a later option.

## File Structure

```text
server/
├── migrations/00007_ai.sql                    settings, chunks, ask_threads, ask_queries
├── internal/
│   ├── config/config.go                       APP_SECRET_KEY
│   ├── migrate/migrate.go                     River's migrator before the grants
│   ├── secret/secret.go (+ test)              AES-GCM seal and open for API keys
│   ├── llm/client.go (+ test)                 OpenAI-compatible models, chat stream, embeddings
│   ├── aisettings/settings.go (+ test)        the `ai` settings row, defaults, validation, 5 s cache
│   ├── jobs/jobs.go                           River client, queues, workers, InsertTx helpers
│   ├── indexer/{chunk,index,embed}.go (+ tests) chunking, hashes, upserts, embedding, re-index
│   ├── ask/{scope,retrieve,pack,prompt,stream,validate,lang}.go (+ tests)
│   ├── db/queries/{settings,chunks,ask}.sql
│   └── httpapi/
│       ├── server.go                          inTx passes the tx; jobs and AI settings wired in
│       ├── ask.go (+ ask_test.go)             POST /ask (SSE and JSON), threads
│       ├── admin_ai.go (+ admin_ai_test.go)   settings, test connection, status, re-index
│       ├── tickets.go, comments.go, decisions.go, nodes.go, clients.go   queue index jobs
│       └── permission_test.go                 Ask evidence and threads in the suite
├── cmd/app/main.go                            River in serve; `app eval`; `app admin reindex --all`
└── eval/{demo-hris.json,golden-v0.jsonl,eval.go} (+ test)
web/
├── app/admin/ai/                              Admin → AI: settings, test, index status
└── e2e/admin-ai.spec.ts                       Off mode and the BYOK acknowledgement
deploy/
├── compose.yaml                               `model` service under the `local-ai` profile; APP_SECRET_KEY
└── compose.host-ai.yaml                       the laptop: Ollama on the host, an egress route for BYOK
```

## Tasks

Each task below lists its files and interfaces. Its steps (failing test, run, implement, run, commit) follow the frame's approval.

### Task 1: Schema for chunks, Ask and settings
- Create `server/migrations/00007_ai.sql`: `settings`, `chunks` (§16, with `embedding halfvec(1024)` nullable and `embed_model`), `ask_threads`, `ask_queries`, their indexes (HNSW with `halfvec_cosine_ops`, GIN on `tsv` and `node_ids`).
- sqlc override: `halfvec` ↔ `pgvector.HalfVector`.
- Queries: settings get/put; chunk upsert, delete by source, list by source, pending-embedding batch, set embedding, counts per model; thread and query log inserts and reads.
- Test: `internal/db/chunks_test.go` (upsert keeps unchanged vectors, delete by source, counts per model).

### Task 2: River in migrate and serve
- `migrate.Up` runs `rivermigrate` (up) after goose and before `ensureAppRole`, so the app role gets River's tables.
- `internal/jobs`: `New(pool, log, workers)` returns the River client with queue `index` (10 workers), max 10 attempts, and `InsertTx(ctx, tx, args)`.
- `httpapi.inTx` passes the `pgx.Tx` alongside the queries: `inTx(ctx, func(q *db.Queries, tx pgx.Tx) error)`; existing handlers change their signature only.
- Test: `migrate_test.go` sees `river_job` and the app role can insert into it.

### Task 3: Secrets and AI settings
- `internal/config`: `APP_SECRET_KEY` (32 bytes, base64), optional; BYOK and remote keys need it.
- `internal/secret`: `Seal(key, plaintext) ([]byte, error)`, `Open(key, sealed) ([]byte, error)`.
- `internal/aisettings`: `Settings{Mode; Chat, Embed Endpoint{URL, Model, APIKeySealed}; Provider; Acknowledged; ContextTokens, MaxConcurrent, Temperature, TimeoutSeconds, MinSimilarity, ExhaustiveMax; EmbedDim}`, `Defaults()` with mode Off and the dev-laptop tier's presets ready for Local, `Validate`, and `Store.Get(ctx)` with a 5-second cache.
- Test: defaults, validation, sealing round trip, cache expiry.

### Task 4: The `llm` client
- `llm.New(Endpoint, apiKey, httpClient)`, `Models(ctx)`, `Embed(ctx, texts) ([][]float32, error)`, `ChatStream(ctx, ChatRequest) (io.ReadCloser, error)` returning the concatenated content deltas of the SSE stream, and a JSON-mode fallback when the provider rejects `json_schema`.
- 429 retries with backoff inside the caller's deadline.
- Test: an `httptest` fake of the OpenAI-compatible API, including a streamed answer split mid-token and a 429.

### Task 5: Admin → AI API
- `GET, PUT /admin/settings/ai` (keys write-only: `api_key_set: true`), `POST /admin/ai/test` (models, one-sentence chat, one embedding, latencies, detected dimension), `GET /admin/ai/status` (pending and failed jobs, chunks per model, last indexed), `POST /admin/ai/reindex` (all, or retry failed).
- R-AI-1: BYOK without `acknowledged` answers 422 `byok_not_acknowledged` and calls nothing; the acknowledgement is an audit event.
- Test: `admin_ai_test.go` with the fake model server; system admins only (403 for project admins).

### Task 6: Chunking
- `indexer.Chunks(src Source) []Chunk` for a ticket header, a comment and a decision record, each starting with a context line such as "HRIS-231 · Client A · Overtime Approval"; splits at about 400 tokens with 50 overlap, tokens estimated as characters ÷ 3.5; content hash per chunk.
- Filter columns: project, client, direct node ids, user and contact ids, internal flag, occurred_at (§13.1).
- Test: table tests for each source, the split and overlap, and the context line.

### Task 7: The index job
- River job `index_source{type, id}`: advisory lock per source, reload, delete chunks of archived or deleted sources, rebuild, keep unchanged hashes (update filter columns only), embed changed chunks in batches of 32 unless mode is Off, upsert.
- River job `embed_pending`: embeds chunks without a vector for the current model, newest first; periodic every minute and queued after a settings change.
- The embedding worker waits while a generation holds the Ask semaphore (§11.7).
- Test: AC-IX-2 (model server down: save succeeds, job retries, drains after), AC-IX-3 (five edits, only the final content, unchanged chunks not re-embedded).

### Task 8: Queue index jobs from every mutation
- Ticket create, update, transition (close and reopen), archive; comment create, edit, delete; decision confirm, draft, edit; node rename or move (its tickets); client rename (its tickets).
- Test: each mutation leaves one `index_source` job in `river_job`, committed with the change; a failed mutation leaves none.

### Task 9: Embedding-model change and re-index all
- A new embedding model or dimension: confirm with chunk count, then a job drops the HNSW index, clears vectors, alters `halfvec(N)`, recreates the index and queues `embed_pending`. `app admin reindex --all` queues every source.
- Test: AC-IX-4 with the fake server switching to 768 dimensions; status reports 100% on the new model afterwards.

### Task 10: Scope detection and language
- `ask.Detect(ctx, q, grants, question) Detected{ClientIDs, NodeIDs, PersonIDs, From, To, Keys}`: clients by name, code, alias (trigram ≥ 0.6 for 4+ letters), only in scope; nodes deepest match; people after cue words; the §11.2 date phrases; ticket keys.
- `ask.Language(question, uiLocale) string`.
- Test: table tests in Indonesian and English, including "Indah" without a cue word and a month without a year.

### Task 11: Retrieval and packing
- `ask.Retrieve(ctx, q, grants, scope, questionVec, question) (Evidence, error)`: SQL filter under visibility and scope; exhaustive when ≤ `exhaustive_max` items; else vector top 50 (`hnsw.iterative_scan = relaxed_order`, `ef_search = 100`) and keyword top 50, RRF (k = 60), group by ticket, top 12; relevance floor.
- `ask.Pack(evidence, budgetTokens) (text string, keys []string)` in the §11.4 layout; over budget, lowest-ranked tickets first, then comments before reason and decision.
- Test: AC-AK-3 (out-of-scope client never retrieved), AC-AK-4 (date range), the floor, the exhaustive path, the budget.

### Task 12: Prompt, streaming decoder and validator
- `ask.Prompt(language, packed)` and `ask.Schema(keys)`.
- `ask.Claims(r io.Reader, emit func(Claim))` emits each claim as its object closes.
- `ask.Validate(claim, evidenceKeys) (Claim, bool)`: drop outside citations, then empty claims, then claims naming an outside key.
- Test: AC-AK-6 cases; a stream cut mid-claim; invalid JSON.

### Task 13: `POST /ask` and threads
- SSE and JSON by `Accept`; events `queued`, `scope`, `evidence`, `claim`, `result`, `error`; a comment every 15 s; the semaphore with 90 s wait; 60 s answer timeout; Off mode keyword results; the query log row; `GET /ask/threads`, `GET, DELETE /ask/threads/{id}` (owner only; DELETE hides).
- Test: AC-AK-1, AC-AK-2, AC-AK-5, AC-AK-7, AC-AK-10 and AC-IX-5 against the fake model server, whose answers are scripted per test.

### Task 14: The permission suite covers Ask
- Every suite user asks a question whose evidence would span every client; each sees only their scope's keys in `evidence` and `claim` events. Threads of another user answer 404.
- Planted leaks in the chunk retrieval queries must fail it.

### Task 15: `app eval`, the demo dataset and golden set v0
- `app eval --set eval/golden-v0.jsonl [--seed eval/demo-hris.json]` seeds a fresh project when asked, indexes it, runs every question under a system admin with fixed seed, and prints citation precision, evidence recall@12, abstention and median latency, with a per-question table; exit status 1 when a target is missed.
- Test: the eval over a three-question set against the fake server computes each measure.

### Task 16: Admin → AI page
- `/admin/ai` (system admins; an "AI" tab in the admin top bar): mode, endpoints and models, API key field (write-only, "Key saved"), the BYOK acknowledgement naming the provider, Test connection with latencies, Index status with Retry and Re-index all, the embedding-change confirmation with chunk count.
- `web/e2e/admin-ai.spec.ts`: Off mode saves; BYOK without the tick shows the refusal (AC-IX-6).

### Task 17: Deploy and the exit check
- `deploy/compose.yaml`: `APP_SECRET_KEY`; a `model` service (Ollama) under the `local-ai` profile on the internal network. `deploy/compose.host-ai.yaml`: `host.docker.internal` for the laptop and an egress network for BYOK.
- On the dev laptop, with the Ollama already installed there (`qwen3.5:4b` and `bge-m3` pulled on 23 Sep 2026, served with `OLLAMA_CONTEXT_LENGTH=8192 OLLAMA_KEEP_ALIVE=-1 ollama serve`): start the stack with the override, set Admin → AI to Local with `http://host.docker.internal:11434/v1`, chat `qwen3.5:4b`, embeddings `bge-m3`, then `docker compose exec app /app eval --seed … --set …`. Expected: citation precision ≥ 90%, recall@12 ≥ 85%, abstention 100%; median latency is recorded (the laptop is expected to miss 15 s, §18.1).
- Update FSD §21: the Iteration 4 row names `qwen3.5:4b` on the dev laptop for the exit check, and Progress records 4a.
