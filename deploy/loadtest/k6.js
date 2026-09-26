// The load test (FSD §18, §21.1): 50 virtual users browse 100,000 tickets
// while 5 ask questions. Targets: Go JSON under 200 ms at p95, ticket pages
// under 1 s, fewer than 1% failed requests. Ask runs on the keyword path, as
// the seeded chunks have no vectors; model latency is measured by `app eval`.
//   k6 run -e BASE=http://localhost -e TICKETS=100000 k6.js   (tokens.txt from seed.sql beside it)
import http from "k6/http";
import { check, sleep } from "k6";

const base = __ENV.BASE || "http://localhost";
const tickets = Number(__ENV.TICKETS || 100000);
const tokens = open("./tokens.txt").trim().split("\n");
const words = ["overtime", "lembur", "payroll", "cuti", "approval", "attendance", "bonus", "tunjangan", "shift", "report"];

export const options = {
  scenarios: {
    browse: { executor: "constant-vus", vus: 50, duration: __ENV.DURATION || "3m", exec: "browse" },
    ask: { executor: "constant-vus", vus: 5, duration: __ENV.DURATION || "3m", exec: "ask" },
  },
  thresholds: {
    "http_req_duration{kind:api}": ["p(95)<200"],
    "http_req_duration{kind:page}": ["p(95)<1000"],
    "http_req_duration{kind:ask}": ["p(95)<3000"],
    http_req_failed: ["rate<0.01"],
  },
};

const pick = (xs) => xs[Math.floor(Math.random() * xs.length)];
const as = (i, kind) => ({ headers: { Cookie: `sid=${tokens[i % tokens.length]}`, Origin: base }, tags: { kind } });

// A member opens the board list, a ticket (JSON and page), its menu's
// timeline and a search, as someone looking up history would.
export function browse() {
  const me = (__VU - 1) % 45; // users 1–45 browse; 46–50 ask
  const n = 1 + Math.floor(Math.random() * tickets);
  const list = http.get(`${base}/api/v1/projects/LOAD/tickets?limit=50&q=${pick(words)}`, as(me, "api"));
  check(list, { "list 200": (r) => r.status === 200 });
  const t = http.get(`${base}/api/v1/tickets/LOAD-${n}`, as(me, "api"));
  check(t, { "ticket 200": (r) => r.status === 200 });
  const page = http.get(`${base}/t/LOAD-${n}`, as(me, "page"));
  check(page, { "page 200": (r) => r.status === 200 });
  if (t.status === 200) {
    const node = t.json("nodes.0.id");
    if (node) check(http.get(`${base}/api/v1/nodes/${node}/timeline?limit=50`, as(me, "api")), { "timeline 200": (r) => r.status === 200 });
  }
  check(http.get(`${base}/api/v1/search?q=${pick(words)}+${pick(words)}`, as(me, "api")), { "search 200": (r) => r.status === 200 });
  sleep(1);
}

// Ask takes 10 questions a minute per user, so each asker waits 7 s.
export function ask() {
  const me = 45 + ((__VU - 1) % 5);
  const r = http.post(
    `${base}/api/v1/ask`,
    JSON.stringify({ question: `Kenapa ${pick(words)} ${pick(words)} berubah?`, scope: {} }),
    { ...as(me, "ask"), headers: { ...as(me, "ask").headers, "Content-Type": "application/json" } },
  );
  check(r, { "ask 200": (x) => x.status === 200 });
  sleep(7);
}
