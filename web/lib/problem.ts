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

type Translate = { (key: string, values?: Record<string, string>): string; has: (key: string) => boolean };

/**
 * An API problem as a sentence, from the "errors" messages. A field error whose
 * code alone would not say which field ("invalid", "required") names the field
 * when its label is known: "Periksa isian Kunci", not "Periksa isian ini".
 */
export function problemMessage(p: Problem | undefined, t: Translate): string {
  const key = problemKey(p);
  const field = p?.errors?.[0]?.field.split(".").pop();
  if ((key === "invalid" || key === "required") && field && t.has(`fields.${field}`)) {
    return t(`${key}Field`, { field: t(`fields.${field}`) });
  }
  return t.has(key) ? t(key) : t("generic");
}

/** Turns an API problem into a sentence in the user's language. */
export function useProblemText() {
  const t = useTranslations("errors");
  return (p?: Problem) => problemMessage(p, t);
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
export type TicketLink = components["schemas"]["TicketLink"];
export type LinkType = components["schemas"]["LinkType"];
export type Note = components["schemas"]["Note"];
export type NoteSummary = components["schemas"]["NoteSummary"];
export type TimelineNote = components["schemas"]["TimelineNote"];
export type AskFeedback = components["schemas"]["AskFeedback"];
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
