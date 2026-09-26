import type { components } from "./api-types";
import { SSEParser } from "./sse";

export type AskItem = components["schemas"]["AskItem"];
export type AskClaim = components["schemas"]["AskClaim"];
export type AskScope = components["schemas"]["AskScope"];
export type AskScopeEvent = components["schemas"]["AskScopeEvent"];
export type AskLabel = components["schemas"]["AskLabel"];
export type AskIgnore = components["schemas"]["AskIgnore"];
export type AskRequest = components["schemas"]["AskRequest"];
export type AskStatus = components["schemas"]["AskResult"]["status"];
export type AskFeedback = components["schemas"]["AskFeedback"];

/** Where a cited key opens: a document section, a decision note such as HRIS-DN7, or a ticket. */
export const itemHref = (key: string) => {
  const doc = /^([A-Z][A-Z0-9]*-DOC\d+)\/(.+)$/i.exec(key); // a document section, e.g. HRIS-DOC1/7.4
  if (doc) return `/documents/${doc[1]}#s-${doc[2]}`;
  return /-DN\d+$/i.test(key) ? `/notes/${key}` : `/t/${key}`;
};

// What the `result` event carries (FSD §11.6).
export type AskDone = {
  status: AskStatus;
  query_id: number;
  thread_id: number;
  language: "id" | "en";
  model?: string | null;
  message?: string | null;
  closest: AskItem[];
  results: AskItem[];
};

export type AskEvent =
  | { type: "queued"; position: number }
  | { type: "scope"; scope: AskScopeEvent }
  | { type: "evidence"; items: AskItem[] }
  | { type: "claim"; claim: AskClaim }
  | { type: "error"; code: string }
  | { type: "result"; result: AskDone };

/**
 * Sends a question and streams the answer as events. A refused request (rate
 * limit, validation, sign-in) resolves to its problem code instead.
 */
export async function askStream(body: AskRequest, onEvent: (e: AskEvent) => void, signal?: AbortSignal): Promise<string | undefined> {
  const res = await fetch("/api/v1/ask", {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    const problem = await res.json().catch(() => undefined);
    return problem?.code ?? "generic";
  }
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  const parser = new SSEParser();
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    for (const ev of parser.push(value)) {
      const data = JSON.parse(ev.data);
      switch (ev.event) {
        case "queued":
          onEvent({ type: "queued", position: data.position });
          break;
        case "scope":
          onEvent({ type: "scope", scope: data });
          break;
        case "evidence":
          onEvent({ type: "evidence", items: data });
          break;
        case "claim":
          onEvent({ type: "claim", claim: data });
          break;
        case "error":
          onEvent({ type: "error", code: data.code });
          break;
        case "result":
          onEvent({ type: "result", result: data });
          break;
      }
    }
  }
  return undefined;
}

/** The answer as markdown with links, for "Copy as markdown" (§10.3). */
export function answerMarkdown(question: string, claims: AskClaim[], origin: string): string {
  const cite = (k: string) => `[${k}](${origin}${itemHref(k)})`;
  return [`**${question}**`, "", ...claims.map((c) => `- ${c.text} ${c.cites.map(cite).join(" ")}`)].join("\n");
}
