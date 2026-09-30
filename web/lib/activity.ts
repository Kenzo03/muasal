import type { ActivityItem } from "./problem";

export type Change = { old?: unknown; new?: unknown };

// The two calls describeChange needs from a next-intl translator of the "activity" messages.
type Translate = { (key: string, values?: Record<string, string>): string; has(key: string): boolean };

/** A value as the history shows it: "—" for nothing, lists joined. */
export const shown = (v: unknown) =>
  v === null || v === undefined || v === "" ? "—" : Array.isArray(v) ? v.join(", ") || "—" : String(v);

/** One history event or comment as a sentence, e.g. "Rina changed Status from To do to In progress". With meId, the reader's own changes read "You". */
export function describeChange(t: Translate, it: ActivityItem, meId?: number): string {
  const actor = it.actor ? (it.actor.id === meId ? t("you") : it.actor.name) : t("system");
  if (it.kind === "comment") return t("commented", { actor });
  const c = (it.changes ?? {}) as Record<string, unknown>;
  const field = (k: string) => (t.has(`fields.${k}`) ? t(`fields.${k}`) : k);
  switch (it.action) {
    case "create":
      return t("created", { actor });
    case "transition": {
      const s = (c.status ?? {}) as Change;
      return t("transition", { actor, old: shown(s.old), new: shown(s.new) });
    }
    case "update":
      return t("updated", { actor, fields: Object.keys(c).map(field).join(", ") });
    case "comment_edit":
      return t("commentEdited", { actor });
    case "comment_delete":
      return t("commentDeleted", { actor });
    case "attachment_add":
      return t("attachmentAdded", { actor, file: shown(c.filename) });
    case "attachment_delete":
      return t("attachmentDeleted", { actor, file: shown(c.filename) });
    case "import_create": // MSL-41: not the raw action name
      return t("imported", { actor, ref: shown(c.external_ref) });
    case "import_update":
      return t("importUpdated", { actor, ref: shown(c.external_ref) });
    case "decision_confirm":
      return t("decisionConfirmed", { actor });
    case "decision_draft":
      return t("decisionDrafted", { actor });
    case "decision_edit":
      return t("decisionEdited", { actor });
    case "link":
    case "unlink":
      // The wording reads from this ticket's side: "Reverses HRIS-88" or "Reversed by HRIS-240" (§8.8).
      return t(it.action === "link" ? "linked" : "unlinked", { actor, link: t(`links.${shown(c.type)}.${c.outgoing ? "out" : "in"}`), key: shown(c.key) });
    default:
      return `${actor}: ${it.action}`;
  }
}
