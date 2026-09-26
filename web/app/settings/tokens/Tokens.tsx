"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { utc } from "@/lib/format";
import { useProblemText } from "@/lib/problem";
import { button, cx, field, panel, table } from "@/lib/ui";

type APIToken = components["schemas"]["APIToken"];

// Personal access tokens for scripts (FSD §14.3): a name, read-only or
// read-write, an optional last day. The secret shows once, right after it is made.
export default function Tokens({ tokens }: { tokens: APIToken[] }) {
  const t = useTranslations("tokens");
  const locale = useLocale();
  const router = useRouter();
  const problemText = useProblemText();
  const [secret, setSecret] = useState("");
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState("");

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    const expires = String(form.get("expires_on") ?? "");
    const { data, error } = await api.POST("/me/tokens", {
      body: { name: String(form.get("name")), read_only: form.get("access") !== "write", expires_on: expires || undefined },
    });
    if (error) return setError(problemText(error));
    setError("");
    setCopied(false);
    setSecret(data!.token);
    formEl.reset();
    router.refresh();
  }

  async function revoke(tok: APIToken) {
    if (!window.confirm(t("revokeConfirm", { name: tok.name }))) return;
    const { error } = await api.DELETE("/me/tokens/{id}", { params: { path: { id: tok.id } } });
    if (error) return setError(problemText(error));
    router.refresh();
  }

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-4">
      <p className="text-[13px] text-muted">{t("intro")}</p>
      {secret && (
        <div role="status" className="flex flex-col gap-2 rounded border border-warn-line bg-warn-soft p-3 text-[13px]">
          <p className="font-semibold text-warn">{t("once")}</p>
          <code className="break-all rounded bg-white px-2 py-1.5 font-mono text-xs" aria-label={t("secret")}>{secret}</code>
          <button
            type="button"
            className={cx(button.secondary, "self-start")}
            onClick={async () => {
              await navigator.clipboard.writeText(secret);
              setCopied(true);
            }}
          >
            {copied ? t("copied") : t("copy")}
          </button>
        </div>
      )}
      <form onSubmit={create} aria-label={t("new")} className={cx(panel, "flex flex-wrap items-end gap-3 p-4")}>
        <label className={field.label}>
          {t("name")}
          <input name="name" required maxLength={100} placeholder={t("namePlaceholder")} className={field.input} />
        </label>
        <label className={field.label}>
          {t("access")}
          <select name="access" defaultValue="read" className={field.input}>
            <option value="read">{t("readOnly")}</option>
            <option value="write">{t("readWrite")}</option>
          </select>
        </label>
        <label className={field.label}>
          {t("expiresOn")}
          <input type="date" name="expires_on" className={field.input} />
        </label>
        <button className={button.primary}>{t("create")}</button>
      </form>
      {error && <p role="alert" className={field.error}>{error}</p>}
      {tokens.length === 0 ? (
        <p className="text-muted">{t("none")}</p>
      ) : (
        <div className={table.wrap}>
          <table className={table.table}>
            <thead className={table.head}>
              <tr>
                <th className={table.th}>{t("name")}</th>
                <th className={table.th}>{t("access")}</th>
                <th className={table.th}>{t("created")}</th>
                <th className={table.th}>{t("lastUsed")}</th>
                <th className={table.th}>{t("expires")}</th>
                <th className={table.th}><span className="sr-only">{t("revoke")}</span></th>
              </tr>
            </thead>
            <tbody>
              {tokens.map((tok) => (
                <tr key={tok.id} className={table.row}>
                  <td className={table.td}>{tok.name}</td>
                  <td className={table.td}>{tok.read_only ? t("readOnly") : t("readWrite")}</td>
                  <td className={table.td}>{utc(tok.created_at, locale)}</td>
                  <td className={table.td}>{tok.last_used_at ? utc(tok.last_used_at, locale) : t("never")}</td>
                  <td className={table.td}>{tok.expires_at ? utc(tok.expires_at, locale) : t("noExpiry")}</td>
                  <td className={table.td}>
                    <button type="button" onClick={() => revoke(tok)} className={cx(button.quiet, "text-danger")}>{t("revoke")}</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="text-xs text-muted">{t("usage")}</p>
    </div>
  );
}
