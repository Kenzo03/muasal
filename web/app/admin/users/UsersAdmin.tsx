"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { problemKey, type Problem, type User } from "@/lib/problem";

// Deterministic on server and browser, so hydration matches (profile timezones arrive in a later iteration).
function utc(iso: string) {
  return `${iso.slice(0, 16).replace("T", " ")} UTC`;
}

export default function UsersAdmin({ users, meId }: { users: User[]; meId: number }) {
  const t = useTranslations("users");
  const tErr = useTranslations("errors");
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

  const input = "rounded border px-3 py-2";
  return (
    <div className="flex flex-col gap-6">
      <form onSubmit={create} className="flex flex-wrap items-end gap-3 rounded-lg border bg-white p-4">
        <label className="flex flex-col gap-1 text-sm">
          {t("name")}
          <input name="name" required maxLength={200} className={input} />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {t("email")}
          <input name="email" type="email" required className={input} />
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input name="is_admin" type="checkbox" />
          {t("admin")}
        </label>
        <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("create")}</button>
      </form>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      {link && (
        <p role="status" className="rounded border border-amber-300 bg-amber-50 p-3 text-sm">
          {t("linkFor", { name: link.name })}{" "}
          <code data-testid="setup-link" className="break-all">{link.url}</code>
        </p>
      )}
      <table className="w-full border-collapse bg-white text-left text-sm">
        <thead>
          <tr className="border-b">
            <th className="p-2">{t("name")}</th>
            <th className="p-2">{t("email")}</th>
            <th className="p-2">{t("admin")}</th>
            <th className="p-2">{t("status")}</th>
            <th className="p-2">{t("lastLogin")}</th>
            <th className="p-2" />
          </tr>
        </thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.id} className="border-b">
              <td className="p-2">{u.name}</td>
              <td className="p-2">{u.email}</td>
              <td className="p-2">{u.is_admin ? "✓" : ""}</td>
              <td className="p-2">{u.disabled ? t("disabled") : t("active")}</td>
              <td className="p-2">{u.last_login_at ? utc(u.last_login_at) : t("never")}</td>
              <td className="flex gap-3 p-2">
                {u.id !== meId && (
                  <>
                    <button type="button" className="underline" onClick={() => setDisabled(u, !u.disabled)}>
                      {u.disabled ? t("enable") : t("disable")}
                    </button>
                    <button type="button" className="underline" onClick={() => resetPassword(u)}>
                      {t("resetPassword")}
                    </button>
                  </>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
