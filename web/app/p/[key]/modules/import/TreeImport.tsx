"use client";

import { useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import type { components } from "@/lib/api-types";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";

type Result = components["schemas"]["NodeImportResult"];

// Module-tree import (FSD §7.5): upload, preview as a tree diff with row
// errors, confirm. Nothing imports while a row is wrong (AC-MR-7).
export default function TreeImport({ projectKey }: { projectKey: string }) {
  const t = useTranslations("treeImport");
  const [file, setFile] = useState<File | null>(null);
  const [plan, setPlan] = useState<Result | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function send(dryRun: boolean) {
    if (!file) return;
    const form = new FormData();
    form.append("file", file);
    if (dryRun) form.append("dry_run", "true");
    setBusy(true);
    const res = await fetch(`/api/v1/projects/${projectKey}/nodes/import`, { method: "POST", body: form });
    setBusy(false);
    const body = await res.json().catch(() => undefined);
    if (!res.ok) return setError(body?.title ?? t("failed"));
    setError("");
    setPlan(body as Result);
  }

  const problems = plan?.problems ?? [];
  return (
    <div className="flex max-w-4xl flex-col gap-4">
      <p className="text-[13px] text-muted">{t("intro")}</p>
      <pre className="overflow-x-auto rounded border border-line bg-paper p-3 font-mono text-xs">{`path,type,code,client_scope,clients,aliases
HR > Attendance,module,HR.ATT,shared,,
HR > Attendance > Overtime Approval,menu,HR.ATT.OT,client_specific,Client A;Client C,Persetujuan Lembur`}</pre>
      <div className={cx(panel, "flex flex-wrap items-end gap-3 p-4")}>
        <label className={field.label}>
          {t("file")}
          <input type="file" accept=".csv,text/csv" onChange={(e) => { setFile(e.target.files?.[0] ?? null); setPlan(null); }} className="text-[13px]" />
        </label>
        <button type="button" disabled={!file || busy} onClick={() => send(true)} className={button.secondary}>{t("preview")}</button>
      </div>
      {error && <p role="alert" className={field.error}>{error}</p>}
      {plan && (
        <section aria-label={t("plan")} className={cx(panel, "flex flex-col gap-3 p-4 text-[13px]")}>
          {plan.applied ? (
            <p role="status" className="font-semibold text-ok">{t("applied", { created: plan.created.length, changed: plan.changed.length })}</p>
          ) : (
            <p className="font-semibold">{t("summary", { created: plan.created.length, changed: plan.changed.length, unchanged: plan.unchanged })}</p>
          )}
          {problems.length > 0 && (
            <div className="flex flex-col gap-1">
              <h2 className={sectionTitle}>{t("problems", { count: problems.length })}</h2>
              <ul className="flex flex-col gap-1 text-danger">
                {problems.map((p, i) => (
                  <li key={i}>{p.line > 0 ? t("row", { line: p.line }) : ""} {t.has(`codes.${p.code}`) ? t(`codes.${p.code}`, { path: p.path ?? "" }) : p.message}</li>
                ))}
              </ul>
            </div>
          )}
          {[["new", plan.created], ["changed", plan.changed]].map(([label, rows]) =>
            (rows as Result["created"]).length > 0 ? (
              <div key={label as string} className="flex flex-col gap-1">
                <h2 className={sectionTitle}>{t(label as "new" | "changed")}</h2>
                <ul className="flex flex-col gap-0.5">
                  {(rows as Result["created"]).map((r) => (
                    <li key={r.line}>
                      <span className="text-muted">{t("row", { line: r.line })}</span> {r.path}
                      {r.fields.length > 0 && <span className="text-muted"> · {r.fields.join(", ")}</span>}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null,
          )}
          {plan.missing.length > 0 && (
            <details>
              <summary className="cursor-pointer text-muted">{t("missing", { count: plan.missing.length })}</summary>
              <ul className="mt-1 flex flex-col gap-0.5">
                {plan.missing.map((m) => <li key={m}>{m}</li>)}
              </ul>
            </details>
          )}
          {!plan.applied && (
            <div className="flex gap-2">
              <button type="button" disabled={busy || problems.length > 0 || plan.created.length + plan.changed.length === 0} onClick={() => send(false)} className={button.primary}>
                {t("apply")}
              </button>
            </div>
          )}
          {plan.applied && <Link href={`/p/${projectKey}/modules`} className="self-start">{t("back")}</Link>}
        </section>
      )}
    </div>
  );
}
