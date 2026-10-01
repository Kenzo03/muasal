// Rules shared by the admin Users and Clients screens: a user's status, search
// over a few fields, and adding a client alias.

export type UserStatus = "active" | "invited" | "disabled";

// MSL-20: a user without a password yet is invited, not active.
export function userStatus(u: { disabled: boolean; has_password: boolean }): UserStatus {
  return u.disabled ? "disabled" : u.has_password ? "active" : "invited";
}

const fold = (s: string) => s.normalize("NFKD").replace(/\p{M}/gu, "").toLocaleLowerCase("id").trim();

// True when the query is empty or any field holds it.
export function matches(query: string, ...fields: (string | null | undefined)[]): boolean {
  const q = fold(query);
  return q === "" || fields.some((f) => f != null && fold(f).includes(q));
}

// The API takes at most 20 aliases per client (ClientUpdate.aliases.maxItems).
export const maxAliases = 20;

// Adds one alias typed in the chip input. It returns the same array when there
// is nothing to add, so callers can skip a state update.
export function addAlias(list: string[], raw: string, max = maxAliases): string[] {
  const alias = raw.replace(/,+\s*$/, "").trim();
  if (alias === "" || list.length >= max || list.some((a) => fold(a) === fold(alias))) return list;
  return [...list, alias];
}
