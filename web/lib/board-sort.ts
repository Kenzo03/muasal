// The orders a board column offers. "priority" is the default and matches
// the server's board order: priority, due date (none last), ticket number.
// Ticket numbers are given in creation order, so they stand for age.
export const boardSorts = ["priority", "priority-low", "newest", "oldest"] as const;
export type BoardSort = (typeof boardSorts)[number];

type Sortable = { key: string; priority: "low" | "medium" | "high" | "urgent"; due_date?: string | null };

const rank = { urgent: 0, high: 1, medium: 2, low: 3 };
const number = (key: string) => Number(key.slice(key.lastIndexOf("-") + 1));
const byDue = (a: Sortable, b: Sortable) =>
  a.due_date === b.due_date ? 0 : !a.due_date ? 1 : !b.due_date ? -1 : a.due_date < b.due_date ? -1 : 1;
const byNumber = (a: Sortable, b: Sortable) => number(a.key) - number(b.key);

export function sortCards<T extends Sortable>(cards: T[], sort: BoardSort): T[] {
  const out = [...cards];
  switch (sort) {
    case "newest":
      return out.sort((a, b) => byNumber(b, a));
    case "oldest":
      return out.sort(byNumber);
    case "priority-low":
      return out.sort((a, b) => rank[b.priority] - rank[a.priority] || byDue(a, b) || byNumber(a, b));
    default:
      return out.sort((a, b) => rank[a.priority] - rank[b.priority] || byDue(a, b) || byNumber(a, b));
  }
}
