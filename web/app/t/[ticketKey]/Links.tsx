"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { StatusDot } from "@/components/Chips";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import { useProblemText, type LinkType, type TicketLink } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";

const types: LinkType[] = ["reverses", "extends", "related_to"];

// Ticket links (FSD §8.8): each link shows on both tickets with inverse
// wording, "Reverses HRIS-88" here and "Reversed by HRIS-240" there. A
// reverses link supersedes the other ticket's decision (R-TK-5).
export default function Links({ ticketKey, links, canEdit }: { ticketKey: string; links: TicketLink[]; canEdit: boolean }) {
  const t = useTranslations("links");
  const router = useRouter();
  const problemText = useProblemText();
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState("");

  async function add(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const { error } = await api.POST("/tickets/{key}/links", {
      params: { path: { key: ticketKey } },
      body: { type: String(form.get("type")) as LinkType, key: String(form.get("key")) },
    });
    if (error) return setError(problemText(error));
    setError("");
    setAdding(false);
    router.refresh();
  }

  async function remove(id: number) {
    const { error } = await api.DELETE("/links/{id}", { params: { path: { id } } });
    if (error) return setError(problemText(error));
    router.refresh();
  }

  return (
    <section aria-labelledby="links-title" className={cx(panel, "flex flex-col gap-2.5 px-4 py-3.5")}>
      <div className="flex items-center gap-2">
        <h2 id="links-title" className={sectionTitle}>{t("title")}</h2>
        {canEdit && !adding && (
          <button type="button" onClick={() => setAdding(true)} className={cx(button.quiet, "ml-auto")}>
            <Icon name="plus" className="size-3.5" />
            {t("add")}
          </button>
        )}
      </div>
      {links.length === 0 && !adding && <p className="text-sm text-muted">{t("none")}</p>}
      {links.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {links.map((l) => (
            <li key={l.id} className="flex flex-wrap items-center gap-2 text-[13px]">
              <span className="text-muted">{t(`${l.type}.${l.outgoing ? "out" : "in"}`)}</span>
              <StatusDot color={l.ticket.status.color} />
              <Link href={`/t/${l.ticket.key}`} className="font-mono font-semibold">{l.ticket.key}</Link>
              <span className="min-w-0 truncate">{l.ticket.title}</span>
              {canEdit && (
                <button type="button" onClick={() => remove(l.id)} aria-label={t("remove", { key: l.ticket.key })} className={cx(button.quiet, "ml-auto text-muted")}>
                  <Icon name="x" className="size-3.5" />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {adding && (
        <form onSubmit={add} aria-label={t("add")} className="flex flex-wrap items-end gap-2">
          <label className={field.label}>
            {t("type")}
            <select name="type" defaultValue="related_to" className={field.compact}>
              {types.map((ty) => (
                <option key={ty} value={ty}>{t(`${ty}.out`)}</option>
              ))}
            </select>
          </label>
          <label className={field.label}>
            {t("key")}
            <input name="key" required placeholder="HRIS-88" className={cx(field.compact, "w-32 font-mono")} />
          </label>
          <button className={button.primary}>{t("save")}</button>
          <button type="button" onClick={() => { setAdding(false); setError(""); }} className={button.secondary}>{t("cancel")}</button>
        </form>
      )}
      {error && <p role="alert" className={field.error}>{error}</p>}
      <p className={field.hint}>{t("hint")}</p>
    </section>
  );
}
