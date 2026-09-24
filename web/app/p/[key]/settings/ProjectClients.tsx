"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client } from "@/lib/problem";

// Which clients the project serves. Project admins may also create a client
// here, since /admin/clients is for system admins (FSD §5.1).
export default function ProjectClients({ projectKey, all, linked }: { projectKey: string; all: Client[]; linked: Client[] }) {
  const t = useTranslations("settings");
  const problemText = useProblemText();
  const router = useRouter();
  const [checked, setChecked] = useState(() => new Set(linked.map((c) => c.id)));
  const [status, setStatus] = useState("");
  const choices = all.filter((c) => !c.archived || checked.has(c.id));

  function toggle(id: number, on: boolean) {
    setChecked((s) => {
      const next = new Set(s);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  async function save(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const { error } = await api.PUT("/projects/{key}/clients", {
      params: { path: { key: projectKey } },
      body: { client_ids: [...checked] },
    });
    if (error) return setStatus(problemText(error));
    setStatus(t("saved"));
    router.refresh(); // member scopes offer the new links
  }

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const { data, error } = await api.POST("/clients", { body: { name: String(new FormData(formEl).get("name")) } });
    if (error) return setStatus(problemText(error));
    formEl.reset();
    toggle(data.id, true);
    setStatus(t("clientCreated", { name: data.name }));
    router.refresh();
  }

  return (
    <section aria-labelledby="clients-title" className="flex flex-col gap-4 rounded-lg border bg-white p-4">
      <h2 id="clients-title" className="font-medium">{t("clientsTitle")}</h2>
      <p className="text-sm text-neutral-600">{t("clientsHint")}</p>
      <form aria-label={t("clientsTitle")} onSubmit={save} className="flex flex-col gap-3">
        {choices.length === 0 && <p className="text-sm text-neutral-600">{t("noClients")}</p>}
        <div className="flex flex-wrap gap-4">
          {choices.map((c) => (
            <label key={c.id} className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={checked.has(c.id)} onChange={(e) => toggle(c.id, e.target.checked)} />
              {c.name}
              {c.archived && <span className="text-xs text-neutral-500">({t("archived")})</span>}
            </label>
          ))}
        </div>
        <button className="self-start rounded bg-neutral-900 px-4 py-2 text-white">{t("saveClients")}</button>
        {status && <p role="status" className="text-sm">{status}</p>}
      </form>
      <form aria-label={t("newClient")} onSubmit={create} className="flex items-end gap-3 border-t pt-4">
        <label className="flex flex-col gap-1 text-sm">
          {t("newClient")}
          <input name="name" required maxLength={200} className="rounded border px-3 py-2" />
        </label>
        <button className="rounded border px-4 py-2">{t("createClient")}</button>
      </form>
    </section>
  );
}
