import type { components } from "./api-types";

export type User = components["schemas"]["User"];
export type Problem = components["schemas"]["Problem"];

/** The translation key for an API error: the first field error's code, else the problem code. */
export function problemKey(p?: Problem): string {
  return p?.errors?.[0]?.code ?? p?.code ?? "generic";
}
