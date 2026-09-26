"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { day } from "@/lib/format";
import { nodePaths } from "@/lib/nodes";
import { useProblemText, type Client, type Node, type Problem } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";

type Scope = components["schemas"]["SummaryScope"];
type Preview = components["schemas"]["SummaryPreview"];

const today = () => new Date().toISOString().slice(0, 10);
const yearStart = () => `${new Date().getFullYear()}-01-01`;

// The summary builder (FSD §12.1): pick the scope, preview the closed tickets
// and notes by menu, untick what should stay out, then generate. Unticked
// items never reach the model.
export default function Builder({ projectKey, clients, nodes, locale }: {
  projectKey: string;
  clients: Client[];
  nodes: Node[];
  locale: "id" | "en";
}) {
  const t = useTranslations("summaries");
  const uiLocale = useLocale();
  const router = useRouter();
  const problemText = useProblemText();
  const pathOf = useMemo(() => nodePaths(nodes), [nodes]);
  const sorted = useMemo(() => [...nodes].sort((a, b) => pathOf(a.id).localeCompare(pathOf(b.id))), [nodes, pathOf]);
  const [scope, setScope] = useState<Scope>({
    project_key: projectKey, node_id: sorted[0]?.id ?? 0, from: yearStart(), to: today(), include_cancelled: false, language: locale, audience: "client",
  });
  const [preview, setPreview] = useState<Preview>();
  const [ticked, setTicked] = useState<Set<string>>(new Set());
  const [problem, setProblem] = useState<Problem>();
  const [busy, setBusy] = useState<"" | "preview" | "generate">("");
  const set = (patch: Partial<Scope>) => {
    setScope((s) => ({ ...s, ...patch }));
    setPreview(undefined);
  };

  async function load(e: React.FormEvent) {
    e.preventDefault();
    setBusy("preview");
    const { data, error } = await api.POST("/summaries/preview", { body: scope });
    setBusy("");
    if (error) return setProblem(error);
    setProblem(undefined);
    setPreview(data);
    setTicked(new Set(data.items.map((i) => i.key)));
  }

  async function generate() {
    setBusy("generate");
    const { data, error } = await api.POST("/summaries", { body: { ...scope, keys: [...ticked] } });
    setBusy("");
    if (error) return setProblem(error);
    router.push(`/summaries/${data.id}`);
  }

  const groups = useMemo(() => {
    const out = new Map<string, Preview["items"]>();
    for (const it of preview?.items ?? []) out.set(it.menu, [...(out.get(it.menu) ?? []), it]);
    return [...out];
  }, [preview]);
  const fieldError = (name: string) => problem?.errors?.find((f) => f.field === name)?.message;

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-4">
      <form onSubmit={load} aria-label={t("scope")} className={cx(panel, "flex flex-wrap items-end gap-3 p-4")}>
        <label className={field.label}>
          {t("node")}
          <select value={scope.node_id} onChange={(e) => set({ node_id: Number(e.target.value) })} className={field.input}>
            {sorted.map((n) => (
              <option key={n.id} value={n.id}>{pathOf(n.id)}</option>
            ))}
          </select>
          {fieldError("node_id") && <span className={field.error}>{fieldError("node_id")}</span>}
        </label>
        <label className={field.label}>
          {t("client")}
          <select value={scope.client_id ?? ""} onChange={(e) => set({ client_id: e.target.value ? Number(e.target.value) : undefined })} className={field.input}>
            <option value="">{t("allClients")}</option>
            {clients.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
        </label>
        <label className={field.label}>
          {t("from")}
          <input type="date" required value={scope.from} onChange={(e) => set({ from: e.target.value })} className={field.input} />
        </label>
        <label className={field.label}>
          {t("to")}
          <input type="date" required value={scope.to} onChange={(e) => set({ to: e.target.value })} className={field.input} />
        </label>
        <label className={field.label}>
          {t("language")}
          <select value={scope.language} onChange={(e) => set({ language: e.target.value as Scope["language"] })} className={field.input}>
            <option value="id">Bahasa Indonesia</option>
            <option value="en">English</option>
          </select>
        </label>
        <label className={field.label}>
          {t("audience")}
          <select value={scope.audience} onChange={(e) => set({ audience: e.target.value as Scope["audience"] })} className={field.input}>
            <option value="client">{t("clientFacing")}</option>
            <option value="internal">{t("internal")}</option>
          </select>
        </label>
        <label className="flex items-center gap-2 text-[13px]">
          <input type="checkbox" checked={scope.include_cancelled ?? false} onChange={(e) => set({ include_cancelled: e.target.checked })} />
          {t("includeCancelled")}
        </label>
        <button disabled={busy !== ""} className={button.secondary}>{busy === "preview" ? t("loading") : t("preview")}</button>
      </form>
      <p className="text-xs text-muted">{scope.audience === "client" ? t("clientNote") : t("internalNote")}</p>
      {problem && !problem.errors?.length && <p role="alert" className={field.error}>{problemText(problem)}</p>}
      {preview && (
        <section aria-labelledby="items-title" className={cx(panel, "flex flex-col gap-3 p-4")}>
          <div className="flex flex-wrap items-center gap-2">
            <h2 id="items-title" className={sectionTitle}>{t("items", { ticked: ticked.size, count: preview.items.length })}</h2>
          </div>
          {preview.items.length === 0 && <p className="text-sm text-muted">{t("noItems")}</p>}
          {groups.map(([menu, items]) => (
            <fieldset key={menu} className="flex flex-col gap-1">
              <legend className="text-[13px] font-semibold">{menu || t("noMenu")}</legend>
              {items.map((it) => (
                <label key={it.key} className="flex items-baseline gap-2 text-[13px]">
                  <input
                    type="checkbox"
                    checked={ticked.has(it.key)}
                    onChange={(e) =>
                      setTicked((s) => {
                        const next = new Set(s);
                        if (e.target.checked) next.add(it.key);
                        else next.delete(it.key);
                        return next;
                      })
                    }
                  />
                  <span className="font-mono text-xs font-semibold">{it.key}</span>
                  <span className="min-w-0 flex-1">{it.title}{it.cancelled ? ` (${t("cancelled")})` : ""}</span>
                  <span className="text-xs text-muted">{day(it.date, uiLocale)}</span>
                </label>
              ))}
            </fieldset>
          ))}
          {preview.items.length > 0 && (
            <div className="flex flex-wrap items-center gap-3 border-t border-line pt-3">
              <p className="text-xs text-muted">
                {preview.model === "" ? t("aiOff") : preview.cloud ? t("sendsCloud", { model: preview.model }) : t("sendsLocal", { model: preview.model })}
              </p>
              <button type="button" onClick={generate} disabled={busy !== "" || ticked.size === 0 || preview.model === ""} className={cx(button.primary, "ml-auto")}>
                {busy === "generate" ? t("generating") : t("generate")}
              </button>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
