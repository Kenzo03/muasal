"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client } from "@/lib/problem";
import { button, cx, field, panel } from "@/lib/ui";

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
    <section aria-labelledby="clients-title" className={cx(panel, "flex flex-col gap-3 p-4")}>
      <div>
        <h2 id="clients-title" className="text-sm font-semibold">{t("clientsTitle")}</h2>
        <p className={field.hint}>{t("clientsHint")}</p>
      </div>
      <form aria-label={t("clientsTitle")} onSubmit={save} className="flex flex-col gap-3">
        {choices.length === 0 && <p className="text-[13px] text-muted">{t("noClients")}</p>}
        <div className="flex flex-wrap gap-2">
          {choices.map((c) => (
            <label
              key={c.id}
              className="flex h-8 items-center gap-2 rounded border border-line px-2.5 text-[13px] has-[:checked]:border-accent has-[:checked]:bg-accent-soft"
            >
              <input type="checkbox" checked={checked.has(c.id)} onChange={(e) => toggle(c.id, e.target.checked)} className="size-4 accent-accent" />
              {c.name}
              {c.archived && <span className="text-xs text-muted">({t("archived")})</span>}
            </label>
          ))}
        </div>
        <div className="flex items-center gap-3">
          <button className={button.primary}>{t("saveClients")}</button>
          {status && <p role="status" className="text-[13px] text-muted">{status}</p>}
        </div>
      </form>
      <form aria-label={t("newClient")} onSubmit={create} className="flex flex-wrap items-end gap-2 border-t border-line-soft pt-3">
        <label className={field.label}>
          {t("newClient")}
          <input name="name" required maxLength={200} className={field.input} />
        </label>
        <button className={cx(button.secondary, "h-[34px]")}>{t("createClient")}</button>
      </form>
    </section>
  );
}
