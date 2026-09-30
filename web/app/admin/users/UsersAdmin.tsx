"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTimeZone, useTranslations } from "next-intl";
import { Avatar } from "@/components/Chips";
import { api } from "@/lib/api";
import { dateTime } from "@/lib/format";
import { problemKey, type Problem, type User } from "@/lib/problem";
import { button, chip, cx, field, panel, table } from "@/lib/ui";

export default function UsersAdmin({ users, meId }: { users: User[]; meId: number }) {
  const t = useTranslations("users");
  const tErr = useTranslations("errors");
  const locale = useLocale();
  const timeZone = useTimeZone();
  const router = useRouter();
  // MSL-20: every link made here stays listed until dismissed, newest first.
  const [links, setLinks] = useState<{ name: string; url: string; emailedTo?: string }[]>([]);
  const [copied, setCopied] = useState("");
  const addLink = (name: string, url: string, emailedTo?: string) => setLinks((ls) => [{ name, url, emailedTo }, ...ls.filter((l) => l.name !== name)]);
  const [error, setError] = useState("");

  function show(p: Problem) {
    const key = problemKey(p);
    setError(tErr.has(key) ? tErr(key) : tErr("generic"));
  }

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    const { data, error } = await api.POST("/admin/users", {
      body: { name: String(form.get("name")), email: String(form.get("email")), is_admin: form.get("is_admin") === "on" },
    });
    if (error) return show(error);
    setError("");
    addLink(data.user.name, data.setup_link.url, data.setup_link.emailed_to);
    formEl.reset();
    router.refresh();
  }

  async function setDisabled(u: User, disabled: boolean) {
    const { error } = await api.PATCH("/admin/users/{id}", { params: { path: { id: u.id } }, body: { disabled } });
    if (error) return show(error);
    router.refresh();
  }

  async function resetPassword(u: User) {
    const { data, error } = await api.POST("/admin/users/{id}/setup-link", { params: { path: { id: u.id } } });
    if (error) return show(error);
    addLink(u.name, data.url, data.emailed_to);
    router.refresh();
  }

  return (
    <div className="flex flex-col gap-4">
      <form onSubmit={create} className={cx(panel, "flex flex-wrap items-end gap-3 p-4")}>
        <label className={field.label}>
          {t("name")}
          <input name="name" required maxLength={200} className={field.input} />
        </label>
        <label className={field.label}>
          {t("email")}
          <input name="email" type="email" required className={cx(field.input, "w-72")} />
        </label>
        <label className="flex h-[34px] items-center gap-2 text-[13px]">
          <input name="is_admin" type="checkbox" className="size-4 accent-accent" />
          {t("admin")}
        </label>
        <button className={cx(button.primary, "h-[34px]")}>{t("create")}</button>
      </form>
      {error && <p role="alert" className={field.error}>{error}</p>}
      {links.map((link) => (
        <p key={link.url} role="status" className="flex flex-wrap items-center gap-x-2 gap-y-1 rounded border border-warn-line bg-warn-soft p-3 text-[13px] text-warn">
          {t("linkFor", { name: link.name })}
          <code data-testid="setup-link" className="min-w-0 flex-1 break-all font-mono text-ink">{link.url}</code>
          <button
            type="button"
            className={button.quiet}
            onClick={() => navigator.clipboard?.writeText(link.url).then(() => setCopied(link.url), () => {})}
          >
            {copied === link.url ? t("copied") : t("copy")}
          </button>
          <button type="button" aria-label={t("dismiss")} className={button.quiet} onClick={() => setLinks((ls) => ls.filter((l) => l.url !== link.url))}>
            ×
          </button>
          {/* MSL-50: with email set up, the link is on its way too. */}
          {link.emailedTo && <span className="basis-full text-xs text-ink-soft">{t("emailedTo", { email: link.emailedTo })}</span>}
        </p>
      ))}
      <div className={table.wrap}>
        <table className={table.table}>
          <thead className={table.head}>
            <tr>
              <th className={table.th}>{t("name")}</th>
              <th className={table.th}>{t("email")}</th>
              <th className={table.th}>{t("admin")}</th>
              <th className={table.th}>{t("status")}</th>
              <th className={table.th}>{t("lastLogin")}</th>
              <th className={table.th} />
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id} className={table.row}>
                <td className={table.td}>
                  <span className="flex items-center gap-2">
                    <Avatar name={u.name} />
                    {u.name}
                  </span>
                </td>
                <td className={table.td}>{u.email}</td>
                <td className={table.td}>{u.is_admin && <span className={cx(chip, "bg-accent-soft text-accent-strong")}>{t("admin")}</span>}</td>
                <td className={table.td}>
                  {/* MSL-20: a user without a password yet is invited, not active. */}
                  <span className={cx(chip, u.disabled ? "bg-well text-muted" : u.has_password ? "bg-ok-soft text-ok" : "bg-warn-soft text-warn")}>
                    {u.disabled ? t("disabled") : u.has_password ? t("active") : t("invited")}
                  </span>
                </td>
                <td className={cx(table.td, "whitespace-nowrap text-muted")}>{u.last_login_at ? dateTime(u.last_login_at, locale, timeZone) : t("never")}</td>
                <td className={cx(table.td, "whitespace-nowrap text-right")}>
                  {u.id !== meId && (
                    <span className="inline-flex gap-3">
                      <button type="button" className={button.quiet} onClick={() => setDisabled(u, !u.disabled)}>
                        {u.disabled ? t("enable") : t("disable")}
                      </button>
                      <button type="button" className={button.quiet} onClick={() => resetPassword(u)}>
                        {u.has_password ? t("resetPassword") : t("newLink")}
                      </button>
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
