"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Member, type ProjectRole } from "@/lib/problem";

type Row = { email: string; name: string; role: ProjectRole; all_clients: boolean; client_ids: number[] };

const toRow = (m: Member): Row => ({ email: m.email, name: m.name, role: m.role, all_clients: m.all_clients, client_ids: m.client_ids });

// Edits the whole member list locally and saves it in one request (FSD §15.2: one by one or in bulk).
export default function ProjectMembers({ projectKey, members, clients }: { projectKey: string; members: Member[]; clients: Client[] }) {
  const t = useTranslations("settings");
  const problemText = useProblemText();
  const [rows, setRows] = useState<Row[]>(() => members.map(toRow));
  const [status, setStatus] = useState("");

  const update = (i: number, patch: Partial<Row>) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)));

  function add(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const email = String(new FormData(e.currentTarget).get("email")).trim();
    setRows((rs) => [...rs, { email, name: email, role: "member", all_clients: true, client_ids: [] }]);
    e.currentTarget.reset();
  }

  async function save() {
    const body = {
      members: rows.map((r) => {
        const all = r.role === "admin" || r.all_clients; // project admins always see every client
        return { email: r.email, role: r.role, all_clients: all, client_ids: all ? [] : r.client_ids };
      }),
    };
    const { data, error } = await api.PUT("/projects/{key}/members", { params: { path: { key: projectKey } }, body });
    if (error) return setStatus(problemText(error));
    setRows(data.items.map(toRow));
    setStatus(t("saved"));
  }

  const input = "rounded border px-2 py-1";
  return (
    <section aria-labelledby="members-title" className="flex flex-col gap-4 rounded-lg border bg-white p-4">
      <h2 id="members-title" className="font-medium">{t("membersTitle")}</h2>
      {rows.length === 0 ? (
        <p className="text-sm text-neutral-600">{t("noMembers")}</p>
      ) : (
        <table className="w-full border-collapse text-left text-sm">
          <thead>
            <tr className="border-b">
              <th className="p-2">{t("member")}</th>
              <th className="p-2">{t("role")}</th>
              <th className="p-2">{t("scope")}</th>
              <th className="p-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={r.email} className="border-b align-top">
                <td className="p-2">
                  {r.name}
                  <div className="text-xs text-neutral-500">{r.email}</div>
                </td>
                <td className="p-2">
                  <select aria-label={t("role")} value={r.role} onChange={(e) => update(i, { role: e.target.value as ProjectRole })} className={input}>
                    <option value="admin">{t("roleAdmin")}</option>
                    <option value="member">{t("roleMember")}</option>
                    <option value="viewer">{t("roleViewer")}</option>
                  </select>
                </td>
                <td className="p-2">
                  {r.role === "admin" ? (
                    <span>{t("allClients")}</span>
                  ) : (
                    <div className="flex flex-col gap-2">
                      <select
                        aria-label={t("scope")}
                        value={r.all_clients ? "all" : "some"}
                        onChange={(e) => update(i, { all_clients: e.target.value === "all" })}
                        className={input}
                      >
                        <option value="all">{t("allClients")}</option>
                        <option value="some">{t("someClients")}</option>
                      </select>
                      {!r.all_clients &&
                        clients.map((c) => (
                          <label key={c.id} className="flex items-center gap-2">
                            <input
                              type="checkbox"
                              checked={r.client_ids.includes(c.id)}
                              onChange={(e) =>
                                update(i, { client_ids: e.target.checked ? [...r.client_ids, c.id] : r.client_ids.filter((id) => id !== c.id) })
                              }
                            />
                            {c.name}
                          </label>
                        ))}
                    </div>
                  )}
                </td>
                <td className="p-2">
                  <button type="button" className="underline" onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}>
                    {t("remove")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <form aria-label={t("addMember")} onSubmit={add} className="flex items-end gap-3">
        <label className="flex flex-col gap-1 text-sm">
          {t("email")}
          <input name="email" type="email" required className="rounded border px-3 py-2" />
        </label>
        <button className="rounded border px-4 py-2">{t("addMember")}</button>
      </form>
      <div className="flex items-center gap-3">
        <button type="button" onClick={save} className="rounded bg-neutral-900 px-4 py-2 text-white">{t("saveMembers")}</button>
        {status && <p role="status" className="text-sm">{status}</p>}
      </div>
    </section>
  );
}
