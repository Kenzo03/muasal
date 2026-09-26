"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { useProblemText } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";

type Run = components["schemas"]["ImportRun"];
type Mapping = components["schemas"]["ImportMapping"];

const fields = ["key", "title", "description", "reason", "type", "status", "priority", "client", "created", "resolved", "reporter", "assignee", "components", "labels", "comments", "attachments"] as const;

// One import (FSD §14.2): map columns, read the dry run (counts, the first 20
// errors, module coverage), then run it in the background and watch progress.
export default function ImportDetail({ initial }: { initial: Run }) {
  const t = useTranslations("imports");
  const problemText = useProblemText();
  const [run, setRun] = useState(initial);
  const [columns, setColumns] = useState<Record<string, string>>(initial.mapping.columns);
  const [values, setValues] = useState(valuesText(initial.mapping));
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const editable = run.status === "uploaded" || run.status === "dry_run" || run.status === "failed";

  // While it runs, poll every two seconds.
  useEffect(() => {
    if (run.status !== "running") return;
    const timer = setInterval(async () => {
      const { data } = await api.GET("/imports/{id}", { params: { path: { id: run.id } } });
      if (data) setRun(data);
    }, 2000);
    return () => clearInterval(timer);
  }, [run.status, run.id]);

  async function plan() {
    let mapping: Mapping;
    try {
      mapping = { columns, ...parseValues(values) };
    } catch {
      return setError(t("badValues"));
    }
    setBusy(true);
    const { data, error } = await api.POST("/imports/{id}/plan", { params: { path: { id: run.id } }, body: mapping });
    setBusy(false);
    if (error) return setError(problemText(error));
    setError("");
    setRun(data!);
  }

  async function start() {
    setBusy(true);
    const { data, error } = await api.POST("/imports/{id}/run", { params: { path: { id: run.id } } });
    setBusy(false);
    if (error) return setError(problemText(error));
    setRun(data!);
  }

  const s = run.stats;
  const valid = s.create + s.update;
  return (
    <>
      <section aria-label={t("dryRun")} className={cx(panel, "grid gap-4 p-4 sm:grid-cols-5")}>
        {([["status", t(`statuses.${run.status}`)], ["create", s.create], ["update", s.update], ["reject", s.reject], ["coverage", `${s.coverage}%`]] as const).map(([k, v]) => (
          <div key={k}>
            <h2 className={sectionTitle}>{t(`stats.${k}`)}</h2>
            <p className="text-[15px] font-semibold">{v}</p>
          </div>
        ))}
        {(run.status === "running" || run.status === "done") && (
          <div className="sm:col-span-5">
            <div className="h-2 overflow-hidden rounded bg-well" role="progressbar" aria-valuemin={0} aria-valuemax={s.rows} aria-valuenow={s.done} aria-label={t("progress")}>
              <div className="h-full bg-accent" style={{ width: `${s.rows ? Math.round((s.done * 100) / s.rows) : 0}%` }} />
            </div>
            <p role="status" className="mt-1 text-[13px] text-muted">
              {run.status === "done" ? t("finished", { tickets: s.tickets, comments: s.comments }) : t("running", { done: s.done, rows: s.rows })}
            </p>
          </div>
        )}
      </section>
      {run.errors.length > 0 && (
        <section aria-label={t("errors")} className={cx(panel, "flex flex-col gap-1 p-4 text-[13px]")}>
          <h2 className={sectionTitle}>{t("errors")}</h2>
          <ul className="flex flex-col gap-0.5 text-danger">
            {run.errors.map((e, i) => (
              <li key={i}>{t("row", { line: e.line })} {e.key ? <span className="font-mono">{e.key}</span> : null} {e.message}</li>
            ))}
          </ul>
        </section>
      )}
      {editable && (
        <section aria-label={t("mapping")} className={cx(panel, "flex flex-col gap-3 p-4")}>
          <h2 className={sectionTitle}>{t("mapping")}</h2>
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
            {fields.map((f) => (
              <label key={f} className={field.label}>
                {t(`fields.${f}`)}
                <select value={columns[f] ?? ""} onChange={(e) => setColumns({ ...columns, [f]: e.target.value })} className={field.compact}>
                  <option value="">{t("none")}</option>
                  {[...new Set(run.headers)].map((h) => (
                    <option key={h} value={h}>{h}</option>
                  ))}
                </select>
              </label>
            ))}
          </div>
          <label className={field.label}>
            {t("values")}
            <textarea value={values} onChange={(e) => setValues(e.target.value)} rows={8} className={cx(field.textarea, "font-mono text-xs")} />
            <span className={field.hint}>{t("valuesHint")}</span>
          </label>
          {error && <p role="alert" className={field.error}>{error}</p>}
          <div className="flex gap-2">
            <button type="button" disabled={busy} onClick={plan} className={button.secondary}>{t("replan")}</button>
            <button type="button" disabled={busy || run.status !== "dry_run" || valid === 0} onClick={start} className={button.primary}>
              {t("run", { count: valid })}
            </button>
          </div>
          <p className={field.hint}>{t("coverageHint")}</p>
        </section>
      )}
      {run.status === "done" && (
        <p className="text-[13px]">
          <Link href={`/p/${run.project_key}/tickets?missing=menus`}>{t("needsLinking")}</Link>
        </p>
      )}
    </>
  );
}

// Value maps as editable lines: "types: story = feature".
function valuesText(m: Mapping): string {
  const lines: string[] = [];
  for (const kind of ["types", "statuses", "priorities", "nodes"] as const) {
    for (const [k, v] of Object.entries(m[kind] ?? {})) lines.push(`${kind}: ${k} = ${v}`);
  }
  return lines.join("\n");
}

function parseValues(text: string): Omit<Mapping, "columns"> {
  const out: { types: Record<string, string>; statuses: Record<string, string>; priorities: Record<string, string>; nodes: Record<string, number> } = {
    types: {}, statuses: {}, priorities: {}, nodes: {},
  };
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    const m = line.match(/^(types|statuses|priorities|nodes):\s*(.+?)\s*=\s*(.+)$/);
    if (!m) throw new Error(line);
    const [, kind, from, to] = m;
    if (kind === "nodes") {
      if (!/^\d+$/.test(to)) throw new Error(line);
      out.nodes[from.toLowerCase()] = Number(to);
    } else out[kind as "types" | "statuses" | "priorities"][from.toLowerCase()] = to;
  }
  return out;
}
