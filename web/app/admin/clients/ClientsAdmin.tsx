"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { ClientChip } from "@/components/Chips";
import { api } from "@/lib/api";
import { useProblemText, type Client } from "@/lib/problem";
import { button, chip, cx, field, panel, table } from "@/lib/ui";

type ClientPatch = { name?: string; code?: string; aliases?: string[]; archived?: boolean };

const aliasList = (v: FormDataEntryValue | null) => String(v ?? "").split(",");

export default function ClientsAdmin({ clients }: { clients: Client[] }) {
  const t = useTranslations("clients");
  const problemText = useProblemText();
  const router = useRouter();
  const [editing, setEditing] = useState<number | null>(null);
  const [error, setError] = useState("");

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    const { error } = await api.POST("/clients", {
      body: { name: String(form.get("name")), code: String(form.get("code")), aliases: aliasList(form.get("aliases")) },
    });
    if (error) return setError(problemText(error));
    setError("");
    formEl.reset();
    router.refresh();
  }

  async function update(c: Client, body: ClientPatch) {
    const { error } = await api.PATCH("/clients/{id}", { params: { path: { id: c.id } }, body });
    if (error) return setError(problemText(error));
    setError("");
    setEditing(null);
    router.refresh();
  }

  return (
    <div className="flex flex-col gap-4">
      <form aria-label={t("create")} onSubmit={create} className={cx(panel, "flex flex-wrap items-end gap-3 p-4")}>
        <label className={field.label}>
          {t("name")}
          <input name="name" required maxLength={200} className={field.input} />
        </label>
        <label className={field.label}>
          {t("code")}
          <input name="code" maxLength={20} className={cx(field.input, "w-28 font-mono")} />
        </label>
        <label className={field.label}>
          {t("aliases")}
          <input name="aliases" className={cx(field.input, "w-64")} />
        </label>
        <button className={cx(button.primary, "h-[34px]")}>{t("create")}</button>
      </form>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <div className={table.wrap}>
        <table className={table.table}>
          <thead className={table.head}>
            <tr>
              <th className={table.th}>{t("name")}</th>
              <th className={table.th}>{t("code")}</th>
              <th className={table.th}>{t("aliasesTitle")}</th>
              <th className={table.th}>{t("status")}</th>
              <th className={table.th} />
            </tr>
          </thead>
          <tbody>
            {clients.map((c) =>
              editing === c.id ? (
                <tr key={c.id} className={table.row}>
                  <td colSpan={5} className={table.td}>
                    <form
                      aria-label={t("editTitle", { name: c.name })}
                      className="flex flex-wrap items-end gap-3"
                      onSubmit={(e) => {
                        e.preventDefault();
                        const form = new FormData(e.currentTarget);
                        update(c, { name: String(form.get("name")), code: String(form.get("code")), aliases: aliasList(form.get("aliases")) });
                      }}
                    >
                      <label className={field.label}>
                        {t("name")}
                        <input name="name" defaultValue={c.name} required maxLength={200} className={field.input} />
                      </label>
                      <label className={field.label}>
                        {t("code")}
                        <input name="code" defaultValue={c.code ?? ""} maxLength={20} className={cx(field.input, "w-28 font-mono")} />
                      </label>
                      <label className={field.label}>
                        {t("aliases")}
                        <input name="aliases" defaultValue={c.aliases.join(", ")} className={cx(field.input, "w-64")} />
                      </label>
                      <button className={cx(button.primary, "h-[34px]")}>{t("save")}</button>
                      <button type="button" onClick={() => setEditing(null)} className={cx(button.secondary, "h-[34px]")}>{t("cancel")}</button>
                    </form>
                  </td>
                </tr>
              ) : (
                <tr key={c.id} className={table.row}>
                  <td className={table.td}><ClientChip client={c} coreLabel="" /></td>
                  <td className={cx(table.td, "font-mono")}>{c.code ?? ""}</td>
                  <td className={table.td}>{c.aliases.join(", ")}</td>
                  <td className={table.td}>
                    <span className={cx(chip, c.archived ? "bg-well text-muted" : "bg-ok-soft text-ok")}>{c.archived ? t("archived") : t("active")}</span>
                  </td>
                  <td className={cx(table.td, "whitespace-nowrap text-right")}>
                    <span className="inline-flex gap-3">
                      <button type="button" className={button.quiet} onClick={() => setEditing(c.id)}>{t("edit")}</button>
                      <button type="button" className={button.quiet} onClick={() => update(c, { archived: !c.archived })}>
                        {c.archived ? t("restore") : t("archive")}
                      </button>
                    </span>
                  </td>
                </tr>
              ),
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
