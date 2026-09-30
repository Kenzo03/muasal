"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { day } from "@/lib/format";
import { useProblemText, type Ref, type Ticket } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";

// Client acceptance (MSL-66): who at the client accepted the ticket's work,
// as in UAT sign-off, and when; members record or remove it.
export default function Acceptance({ ticket, canEdit }: { ticket: Ticket; canEdit: boolean }) {
  const t = useTranslations("acceptance");
  const locale = useLocale();
  const router = useRouter();
  const problemText = useProblemText();
  const [contacts, setContacts] = useState<Ref[]>();
  const [error, setError] = useState("");
  const a = ticket.acceptance;
  const path = { params: { path: { key: ticket.key } } };

  async function open() {
    // The ticket's client's contacts; any client's for core work.
    const { data } = await api.GET("/contacts", { params: { query: ticket.client ? { client_id: ticket.client.id } : {} } });
    setContacts((data?.items ?? []).filter((c) => c.client_id !== null).map((c) => ({ id: c.id, name: ticket.client ? c.name : `${c.name} (${c.client_name})` })));
  }
  async function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const { error } = await api.PUT("/tickets/{key}/acceptance", {
      ...path,
      body: { contact_id: Number(form.get("contact_id")), accepted_on: String(form.get("accepted_on")), note: String(form.get("note") ?? "") },
    });
    if (error) return setError(problemText(error));
    setContacts(undefined);
    router.refresh();
  }
  async function remove() {
    const { error } = await api.DELETE("/tickets/{key}/acceptance", path);
    if (error) return setError(problemText(error));
    router.refresh();
  }

  return (
    <section aria-labelledby="acceptance-title" className={cx(panel, "flex flex-col gap-2 px-4 py-3.5 text-[13px]")}>
      <h2 id="acceptance-title" className={sectionTitle}>{t("title")}</h2>
      {a ? (
        <>
          <p className="font-semibold text-ok">{t("accepted", { name: a.contact.name, date: day(a.accepted_on, locale) })}</p>
          {a.note && <p className="text-muted">{a.note}</p>}
          {canEdit && <button type="button" onClick={remove} className={cx(button.quiet, "self-start")}>{t("remove")}</button>}
        </>
      ) : contacts ? (
        <form onSubmit={save} className="flex flex-col gap-2">
          {contacts.length === 0 ? (
            <p className="text-muted">{t("noContacts")}</p>
          ) : (
            <>
              <label htmlFor="acceptance-contact" className={field.label}>
                {t("contact")}
                <select id="acceptance-contact" name="contact_id" required defaultValue="" className={field.compact}>
                  <option value="" disabled>{t("pick")}</option>
                  {contacts.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
              </label>
              <label htmlFor="acceptance-date" className={field.label}>
                {t("date")}
                <input id="acceptance-date" type="date" name="accepted_on" required max={new Date().toLocaleDateString("en-CA")} defaultValue={new Date().toLocaleDateString("en-CA")} className={field.compact} />
              </label>
              <label htmlFor="acceptance-note" className={field.label}>
                {t("note")}
                <input id="acceptance-note" name="note" maxLength={2000} placeholder={t("notePlaceholder")} className={field.compact} />
              </label>
            </>
          )}
          <div className="flex gap-2">
            {contacts.length > 0 && <button className={button.secondary}>{t("save")}</button>}
            <button type="button" onClick={() => setContacts(undefined)} className={button.quiet}>{t("cancel")}</button>
          </div>
        </form>
      ) : (
        <>
          <p className="text-muted">{t("none")}</p>
          {canEdit && <button type="button" onClick={open} className={cx(button.quiet, "self-start")}>{t("record")}</button>}
        </>
      )}
      {error && <p role="alert" className={field.error}>{error}</p>}
    </section>
  );
}
