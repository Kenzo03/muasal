"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Avatar } from "@/components/Chips";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Member, type ProjectRole } from "@/lib/problem";
import { button, cx, field, panel, table } from "@/lib/ui";

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

  return (
    <section aria-labelledby="members-title" className={cx(panel, "flex flex-col gap-3 p-4")}>
      <h2 id="members-title" className="text-sm font-semibold">{t("membersTitle")}</h2>
      {rows.length === 0 ? (
        <p className="text-[13px] text-muted">{t("noMembers")}</p>
      ) : (
        <div className="overflow-x-auto rounded border border-line-soft">
          <table className={table.table}>
            <thead className={table.head}>
              <tr>
                <th className={table.th}>{t("member")}</th>
                <th className={table.th}>{t("role")}</th>
                <th className={table.th}>{t("scope")}</th>
                <th className={table.th} />
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={r.email} className={table.row}>
                  <td className={table.td}>
                    <span className="flex items-center gap-2">
                      <Avatar name={r.name} />
                      <span>
                        {r.name}
                        <span className="block text-xs text-muted">{r.email}</span>
                      </span>
                    </span>
                  </td>
                  <td className={table.td}>
                    <select aria-label={t("role")} value={r.role} onChange={(e) => update(i, { role: e.target.value as ProjectRole })} className={field.compact}>
                      <option value="admin">{t("roleAdmin")}</option>
                      <option value="member">{t("roleMember")}</option>
                      <option value="viewer">{t("roleViewer")}</option>
                    </select>
                  </td>
                  <td className={table.td}>
                    {r.role === "admin" ? (
                      <span className="text-muted">{t("allClients")}</span>
                    ) : (
                      <div className="flex flex-col gap-2">
                        <select
                          aria-label={t("scope")}
                          value={r.all_clients ? "all" : "some"}
                          onChange={(e) => update(i, { all_clients: e.target.value === "all" })}
                          className={cx(field.compact, "self-start")}
                        >
                          <option value="all">{t("allClients")}</option>
                          <option value="some">{t("someClients")}</option>
                        </select>
                        {!r.all_clients && (
                          <div className="flex flex-wrap gap-x-3 gap-y-1.5">
                            {clients.map((c) => (
                              <label key={c.id} className="flex items-center gap-1.5">
                                <input
                                  type="checkbox"
                                  checked={r.client_ids.includes(c.id)}
                                  onChange={(e) =>
                                    update(i, { client_ids: e.target.checked ? [...r.client_ids, c.id] : r.client_ids.filter((id) => id !== c.id) })
                                  }
                                  className="size-4 accent-accent"
                                />
                                {c.name}
                              </label>
                            ))}
                          </div>
                        )}
                      </div>
                    )}
                  </td>
                  <td className={cx(table.td, "text-right")}>
                    <button type="button" className={button.quiet} onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}>
                      {t("remove")}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <form aria-label={t("addMember")} onSubmit={add} className="flex flex-wrap items-end gap-2">
        <label className={field.label}>
          {t("email")}
          <input name="email" type="email" required className={cx(field.input, "w-72")} />
        </label>
        <button className={cx(button.secondary, "h-[34px]")}>{t("addMember")}</button>
      </form>
      <div className="flex items-center gap-3 border-t border-line-soft pt-3">
        <button type="button" onClick={save} className={button.primary}>{t("saveMembers")}</button>
        {status && <p role="status" className="text-[13px] text-muted">{status}</p>}
      </div>
    </section>
  );
}
