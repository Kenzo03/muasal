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
  const [link, setLink] = useState<{ name: string; url: string } | null>(null);
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
    setLink({ name: data.user.name, url: data.setup_link.url });
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
    setLink({ name: u.name, url: data.url });
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
      {link && (
        <p role="status" className="rounded border border-warn-line bg-warn-soft p-3 text-[13px] text-warn">
          {t("linkFor", { name: link.name })}{" "}
          <code data-testid="setup-link" className="break-all font-mono text-ink">{link.url}</code>
        </p>
      )}
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
                  <span className={cx(chip, u.disabled ? "bg-well text-muted" : "bg-ok-soft text-ok")}>{u.disabled ? t("disabled") : t("active")}</span>
                </td>
                <td className={cx(table.td, "whitespace-nowrap text-muted")}>{u.last_login_at ? dateTime(u.last_login_at, locale, timeZone) : t("never")}</td>
                <td className={cx(table.td, "whitespace-nowrap text-right")}>
                  {u.id !== meId && (
                    <span className="inline-flex gap-3">
                      <button type="button" className={button.quiet} onClick={() => setDisabled(u, !u.disabled)}>
                        {u.disabled ? t("enable") : t("disable")}
                      </button>
                      <button type="button" className={button.quiet} onClick={() => resetPassword(u)}>
                        {t("resetPassword")}
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
