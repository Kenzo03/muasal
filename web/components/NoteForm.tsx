"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import NodePicker from "./NodePicker";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Node, type Note } from "@/lib/problem";
import { button, cx, field } from "@/lib/ui";

// The decision note form (FSD §9.4): a decision made in a meeting, a call or an
// email, dated, on menus or the whole project (MSL-59), with the body
// prefilled with Decision, Why and Alternatives rejected.
export default function NoteForm({ projectKey, note, clients, nodes }: { projectKey: string; note?: Note; clients: Client[]; nodes: Node[] }) {
  const t = useTranslations("notes");
  const locale = useLocale();
  const router = useRouter();
  const problemText = useProblemText();
  const [selected, setSelected] = useState(new Set(note?.nodes.map((n) => n.id) ?? []));
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  // MSL-11: action items get their own section; each one can become a ticket.
  const template = locale === "id"
    ? "## Keputusan\n\n\n## Alasan\n\n\n## Alternatif yang ditolak\n\n\n## Tindak lanjut\n"
    : "## Decision\n\n\n## Why\n\n\n## Alternatives rejected\n\n\n## Action items\n";

  async function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const client = String(form.get("client_id") ?? "");
    const body = {
      title: String(form.get("title")),
      decided_on: String(form.get("decided_on")),
      client_id: client ? Number(client) : undefined,
      attendees: String(form.get("attendees") ?? ""),
      node_ids: [...selected],
      ticket_keys: String(form.get("ticket_keys") ?? "").split(/[\s,]+/).filter(Boolean),
      body: String(form.get("body")),
    };
    setSaving(true);
    const res = note
      ? await api.PATCH("/notes/{noteKey}", { params: { path: { noteKey: note.key } }, body: { ...body, archived: form.get("archived") === "on" } })
      : await api.POST("/projects/{key}/notes", { params: { path: { key: projectKey } }, body });
    setSaving(false);
    if (res.error) return setError(problemText(res.error));
    router.push(`/notes/${res.data!.key}`);
    router.refresh();
  }

  return (
    <form onSubmit={save} aria-label={note ? t("editTitle") : t("newTitle")} className="flex flex-col gap-3.5 p-4">
      <label className={field.label}>
        {t("title")}
        <input name="title" required minLength={5} maxLength={200} defaultValue={note?.title} className={field.input} />
      </label>
      <div className="grid gap-3.5 md:grid-cols-3">
        <label className={field.label}>
          {t("decidedOn")}
          <input type="date" name="decided_on" required defaultValue={note?.decided_on} className={field.input} />
        </label>
        <label className={field.label}>
          {t("client")}
          <select name="client_id" defaultValue={note?.client?.id ?? ""} className={field.input}>
            <option value="">{t("allClients")}</option>
            {clients.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
        </label>
        <label className={field.label}>
          {t("attendees")}
          <input name="attendees" maxLength={500} defaultValue={note?.attendees} placeholder={t("attendeesHint")} className={field.input} />
        </label>
      </div>
      <div className={field.label}>
        {t("menus")}
        <NodePicker nodes={nodes} selected={selected} legend={t("menus")} onToggle={(id, on) => {
          const next = new Set(selected);
          if (on) next.add(id);
          else next.delete(id);
          setSelected(next);
        }} />
        <span className={field.hint}>{t("menusHint")}</span>
      </div>
      <label className={field.label}>
        {t("tickets")}
        <input name="ticket_keys" defaultValue={note?.tickets.map((tk) => tk.key).join(", ")} placeholder={`${projectKey}-12, ${projectKey}-15`} className={cx(field.input, "font-mono")} />
        <span className={field.hint}>{t("ticketsHint")}</span>
      </label>
      <label className={field.label}>
        {t("body")}
        <textarea name="body" required rows={10} maxLength={50000} defaultValue={note?.body ?? template} className={field.textarea} />
        <span className={field.hint}>{t("bodyHint")}</span>
      </label>
      {note && (
        <label className="flex items-center gap-2 text-[13px]">
          <input type="checkbox" name="archived" defaultChecked={note.archived} className="size-4 accent-accent" />
          {t("archived")}
        </label>
      )}
      {error && <p role="alert" className={field.error}>{error}</p>}
      <div className="flex gap-2">
        <button disabled={saving} className={button.primary}>{t("save")}</button>
        <button type="button" onClick={() => router.back()} className={button.secondary}>{t("cancel")}</button>
      </div>
    </form>
  );
}
