"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { useProblemText } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle, table } from "@/lib/ui";

type Repo = components["schemas"]["Repo"];
type Provider = components["schemas"]["RepoProvider"];

const providers: Provider[] = ["github", "gitlab", "gitea"];

// Git repositories (FSD §14.1): each gets a webhook URL and a secret to paste
// into the Git server. The secret shows once, when it is made or replaced.
export default function Repos({ projectKey, repos }: { projectKey: string; repos: Repo[] }) {
  const t = useTranslations("repos");
  const router = useRouter();
  const problemText = useProblemText();
  const [shown, setShown] = useState<Repo | null>(null);
  const [error, setError] = useState("");

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    const { data, error } = await api.POST("/projects/{key}/repos", {
      params: { path: { key: projectKey } },
      body: { provider: String(form.get("provider")) as Provider, name: String(form.get("name")), web_url: String(form.get("web_url")) },
    });
    if (error) return setError(problemText(error));
    setError("");
    setShown(data!);
    formEl.reset();
    router.refresh();
  }

  async function newSecret(r: Repo) {
    if (!window.confirm(t("newSecretConfirm", { name: r.name }))) return;
    const { data, error } = await api.PATCH("/repos/{id}", {
      params: { path: { id: r.id } },
      body: { name: r.name, web_url: r.web_url, new_secret: true },
    });
    if (error) return setError(problemText(error));
    setError("");
    setShown(data!);
  }

  async function remove(r: Repo) {
    if (!window.confirm(t("removeConfirm", { name: r.name }))) return;
    const { error } = await api.DELETE("/repos/{id}", { params: { path: { id: r.id } } });
    if (error) return setError(problemText(error));
    if (shown?.id === r.id) setShown(null);
    router.refresh();
  }

  return (
    <section aria-labelledby="repos-title" className={cx(panel, "flex flex-col gap-3 p-4")}>
      <h2 id="repos-title" className={sectionTitle}>{t("title")}</h2>
      <p className="text-[13px] text-muted">{t("intro")}</p>
      {shown?.secret && (
        <div role="status" className="flex flex-col gap-2 rounded border border-warn-line bg-warn-soft p-3 text-[13px]">
          <p className="font-semibold text-warn">{t("once", { name: shown.name })}</p>
          <p>{t(`howTo.${shown.provider}`)}</p>
          <span className="text-muted">{t("webhookUrl")}</span>
          <code className="break-all rounded bg-white px-2 py-1.5 font-mono text-xs" aria-label={t("webhookUrl")}>{shown.webhook_url}</code>
          <span className="text-muted">{t("secret")}</span>
          <code className="break-all rounded bg-white px-2 py-1.5 font-mono text-xs" aria-label={t("secret")}>{shown.secret}</code>
          <button type="button" className={cx(button.secondary, "self-start")} onClick={() => setShown(null)}>{t("done")}</button>
        </div>
      )}
      {repos.length === 0 ? (
        <p className="text-sm text-muted">{t("none")}</p>
      ) : (
        <div className={table.wrap}>
          <table className={table.table}>
            <thead className={table.head}>
              <tr>
                <th className={table.th}>{t("name")}</th>
                <th className={table.th}>{t("provider")}</th>
                <th className={table.th}>{t("webhookUrl")}</th>
                <th className={table.th}><span className="sr-only">{t("actions")}</span></th>
              </tr>
            </thead>
            <tbody>
              {repos.map((r) => (
                <tr key={r.id} className={table.row}>
                  <td className={table.td}>
                    <a href={r.web_url} target="_blank" rel="noreferrer" className="font-medium">{r.name}</a>
                  </td>
                  <td className={table.td}>{t(`providers.${r.provider}`)}</td>
                  <td className={cx(table.td, "break-all font-mono text-xs")}>{r.webhook_url}</td>
                  <td className={cx(table.td, "whitespace-nowrap")}>
                    <button type="button" onClick={() => newSecret(r)} className={button.quiet}>{t("newSecret")}</button>
                    <button type="button" onClick={() => remove(r)} className={cx(button.quiet, "text-danger")}>{t("remove")}</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <form onSubmit={create} aria-label={t("add")} className="flex flex-wrap items-end gap-3">
        <label className={field.label}>
          {t("provider")}
          <select name="provider" defaultValue="github" className={field.input}>
            {providers.map((p) => (
              <option key={p} value={p}>{t(`providers.${p}`)}</option>
            ))}
          </select>
        </label>
        <label className={field.label}>
          {t("name")}
          <input name="name" required maxLength={200} placeholder="acme/hris" className={field.input} />
        </label>
        <label className={field.label}>
          {t("webUrl")}
          <input name="web_url" type="url" required placeholder="https://github.com/acme/hris" className={field.input} />
        </label>
        <button className={button.primary}>{t("add")}</button>
      </form>
      {error && <p role="alert" className={field.error}>{error}</p>}
    </section>
  );
}
