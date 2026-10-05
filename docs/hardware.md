# Hardware guide

Which server to buy, or rent, for Zettra (FSD §18.1, §21 Iteration 6). Zettra runs on one Linux server with Docker. How fast Ask answers depends almost entirely on the model server. Everything else is modest.

## Pick a tier

| Tier | For | CPU and memory | GPU | Disk | Chat model | Ask median |
| --- | --- | --- | --- | --- | --- | --- |
| AI off | Tickets, history and keyword search only | 4 cores, 8 GB | none | 100 GB SSD | none | keyword results in under a second |
| Minimum | A small team trying Ask | 8+ cores, 32 GB | none | 250 GB SSD | `qwen3.5:4b` | 45–120 s (estimate) |
| Recommended | Daily Ask for a team of up to 50 | 8+ cores, 32 GB | 12–16 GB VRAM (e.g. RTX 4060 Ti 16 GB, RTX A4000) | 250 GB SSD | `qwen3.5:9b` | 15 s or less (target) |
| Large team | Heavy Ask use, many at once | 16+ cores, 64 GB | 24 GB+ VRAM (vLLM) | 500 GB SSD | Qwen3.5-27B or -35B-A3B | 10 s or less (estimate) |

- Embeddings use `bge-m3` (1.2 GB) on every AI tier.
- The dev laptop (MacBook Air M4, 16 GB) runs `qwen3.5:4b` natively in about 18–21 s per answer. It is for development and demos, not for a team.
- Bring your own key (BYOK) needs no GPU: the provider runs the model. Size the server like the AI-off tier.

## Memory

PostgreSQL gets `shared_buffers` and `effective_cache_size` from `.env`:

| Server memory | `PG_SHARED_BUFFERS` | `PG_EFFECTIVE_CACHE_SIZE` |
| --- | --- | --- |
| 8–16 GB | 512MB | 2GB |
| 32 GB | 4GB | 16GB |
| 64 GB | 8GB | 32GB |

`install.sh` writes the 32 GB values on a host with 30 GB or more, and the small ones otherwise.

With local AI on a CPU-only server, the model shares this memory: `qwen3.5:4b` takes about 3.4 GB and `bge-m3` about 0.7 GB.

## Disk

At 100,000 tickets the database is about 8 GB with vectors (estimate, FSD §16):
- half-precision vectors, about 2 GB;
- the HNSW index, 2–3 GB;
- text and other tables, about 3 GB.

Plan for:
- attachments: whatever your teams upload (screenshots are typically 100–500 KB each);
- backups: 14 daily dumps without vectors, plus a copy of every attachment;
- models: 8 GB for the minimum tier, 12 GB for recommended.

Admin → System status warns when the attachments or backups disk reaches 80%.

## Measured

The load test, `deploy/loadtest/run.sh 100000`, ran on the development container: 4 vCPU and 15 GB, with every service and k6 on one host and AI off. It uses 100,000 tickets (1,000,000 chunks), 50 users browsing and 5 asking, for 3 minutes:

| Measure | Target (FSD §18) | Result |
| --- | --- | --- |
| API p95 | under 200 ms | 142 ms |
| Ticket page p95 | under 1 s | 680 ms |
| Ask median, keyword path | under 15 s | 0.25 s |
| Failed requests | under 1% | 0% |

Before Iteration 6, the API p95 was 313 ms and the page p95 1.03 s. Ask's keyword search ranked every chunk that held any word of the question, about 136,000 rows and 3 s of database time for a common word. It now ranks the chunks with all the words first, and at most 5,000 candidates per search.

The Ask figure leaves out the model: add the chat model's time from the tier table above, as measured by `app eval`. Run the test on your own server before a pilot. It seeds project LOAD in a scratch install and prints each measure against its target.

## Network

- Local or off AI: only ports 80 and 443 in. Nothing goes out after the install.
- BYOK: outbound HTTPS to the provider only.
- SSO (P1): outbound HTTPS to the identity provider.
