"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client } from "@/lib/problem";

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

  const input = "rounded border px-3 py-2";
  return (
    <div className="flex flex-col gap-6">
      <form aria-label={t("create")} onSubmit={create} className="flex flex-wrap items-end gap-3 rounded-lg border bg-white p-4">
        <label className="flex flex-col gap-1 text-sm">
          {t("name")}
          <input name="name" required maxLength={200} className={input} />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {t("code")}
          <input name="code" maxLength={20} className={`${input} w-28 font-mono`} />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {t("aliases")}
          <input name="aliases" className={input} />
        </label>
        <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("create")}</button>
      </form>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      <table className="w-full border-collapse bg-white text-left text-sm">
        <thead>
          <tr className="border-b">
            <th className="p-2">{t("name")}</th>
            <th className="p-2">{t("code")}</th>
            <th className="p-2">{t("aliasesTitle")}</th>
            <th className="p-2">{t("status")}</th>
            <th className="p-2" />
          </tr>
        </thead>
        <tbody>
          {clients.map((c) =>
            editing === c.id ? (
              <tr key={c.id} className="border-b">
                <td colSpan={5} className="p-2">
                  <form
                    aria-label={t("editTitle", { name: c.name })}
                    className="flex flex-wrap items-end gap-3"
                    onSubmit={(e) => {
                      e.preventDefault();
                      const form = new FormData(e.currentTarget);
                      update(c, { name: String(form.get("name")), code: String(form.get("code")), aliases: aliasList(form.get("aliases")) });
                    }}
                  >
                    <label className="flex flex-col gap-1 text-sm">
                      {t("name")}
                      <input name="name" defaultValue={c.name} required maxLength={200} className={input} />
                    </label>
                    <label className="flex flex-col gap-1 text-sm">
                      {t("code")}
                      <input name="code" defaultValue={c.code ?? ""} maxLength={20} className={`${input} w-28 font-mono`} />
                    </label>
                    <label className="flex flex-col gap-1 text-sm">
                      {t("aliases")}
                      <input name="aliases" defaultValue={c.aliases.join(", ")} className={input} />
                    </label>
                    <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
                    <button type="button" onClick={() => setEditing(null)} className="rounded border px-4 py-2">{t("cancel")}</button>
                  </form>
                </td>
              </tr>
            ) : (
              <tr key={c.id} className="border-b">
                <td className="p-2">{c.name}</td>
                <td className="p-2 font-mono">{c.code ?? ""}</td>
                <td className="p-2">{c.aliases.join(", ")}</td>
                <td className="p-2">{c.archived ? t("archived") : t("active")}</td>
                <td className="flex gap-3 p-2">
                  <button type="button" className="underline" onClick={() => setEditing(c.id)}>{t("edit")}</button>
                  <button type="button" className="underline" onClick={() => update(c, { archived: !c.archived })}>
                    {c.archived ? t("restore") : t("archive")}
                  </button>
                </td>
              </tr>
            ),
          )}
        </tbody>
      </table>
    </div>
  );
}
