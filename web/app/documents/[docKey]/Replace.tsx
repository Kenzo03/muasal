"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle } from "@/lib/ui";

// Version (§7.7): project admins mark this document replaced by a newer one of
// the project, when its upload did not say so, or make it current again. Ask
// then uses a replaced document only as history.
export default function Replace({ docKey, supersededBy, candidates }: {
  docKey: string;
  supersededBy?: string;
  candidates: { key: string; title: string }[]; // the project's other current documents
}) {
  const t = useTranslations("documents");
  const router = useRouter();
  const problemText = useProblemText();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function save(by: string | null) {
    setBusy(true);
    const { error } = await api.PATCH("/documents/{key}", { params: { path: { key: docKey } }, body: { superseded_by: by } });
    setBusy(false);
    if (error) return setError(problemText(error));
    setError("");
    router.refresh();
  }

  return (
    <section aria-labelledby="version-title" className={cx(panel, "flex flex-col gap-2.5 p-4")}>
      <h2 id="version-title" className={sectionTitle}>{t("version")}</h2>
      {supersededBy ? (
        <>
          <p className="text-[13px] text-ink-soft">{t("supersededBy", { key: supersededBy })}</p>
          <button type="button" disabled={busy} onClick={() => save(null)} className={button.secondary}>{t("makeCurrent")}</button>
        </>
      ) : candidates.length === 0 ? (
        <p className="text-[13px] text-muted">{t("noNewer")}</p>
      ) : (
        <form
          className="flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            save(String(new FormData(e.currentTarget).get("by")));
          }}
        >
          <p className={field.hint}>{t("replaceHint")}</p>
          <label className={field.label}>
            {t("replacedBy")}
            <select name="by" className={field.compact}>
              {candidates.map((c) => (
                <option key={c.key} value={c.key}>{c.key} · {c.title}</option>
              ))}
            </select>
          </label>
          <button disabled={busy} className={button.secondary}>{t("replace")}</button>
        </form>
      )}
      {error && <p role="alert" className={field.error}>{error}</p>}
    </section>
  );
}
