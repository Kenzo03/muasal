import { useTranslations } from "next-intl";
import type { components } from "./api-types";

export type User = components["schemas"]["User"];
export type Problem = components["schemas"]["Problem"];
export type Project = components["schemas"]["Project"];
export type ProjectRole = components["schemas"]["ProjectRole"];
export type Client = components["schemas"]["Client"];
export type Member = components["schemas"]["Member"];
export type Node = components["schemas"]["Node"];

/** The translation key for an API error: the first field error's code, else the problem code. */
export function problemKey(p?: Problem): string {
  return p?.errors?.[0]?.code ?? p?.code ?? "generic";
}

/** Turns an API problem into a sentence in the user's language. */
export function useProblemText() {
  const t = useTranslations("errors");
  return (p?: Problem) => {
    const key = problemKey(p);
    return t.has(key) ? t(key) : t("generic");
  };
}
export type Status = components["schemas"]["Status"];
export type Ticket = components["schemas"]["Ticket"];
export type TicketSummary = components["schemas"]["TicketSummary"];
export type TicketType = components["schemas"]["TicketType"];
export type Priority = components["schemas"]["Priority"];
export type ActivityItem = components["schemas"]["ActivityItem"];
export type Ref = components["schemas"]["Ref"];
export type Contact = components["schemas"]["Contact"];
export type DecisionRecord = components["schemas"]["DecisionRecord"];
export type TimelineEntry = components["schemas"]["TimelineEntry"];
export type Behavior = components["schemas"]["Behavior"];
export type NodeDetail = components["schemas"]["NodeDetail"];
export type SearchResults = components["schemas"]["SearchResults"];
export type MyTicket = components["schemas"]["MyTicket"];
export type RecentTicket = components["schemas"]["RecentTicket"];
export type AIMode = components["schemas"]["AIMode"];
export type AISettings = components["schemas"]["AISettings"];
export type AITestResult = components["schemas"]["AITestResult"];
export type IndexStatus = components["schemas"]["IndexStatus"];
