"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Avatar } from "@/components/Chips";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { useProblemText, type Client, type Member, type ProjectRole } from "@/lib/problem";
import { button, cx, field, panel, table } from "@/lib/ui";

type Row = { email: string; name: string; role: ProjectRole; all_clients: boolean; client_ids: number[] };

const toRow = (m: Member): Row => ({ email: m.email, name: m.name, role: m.role, all_clients: m.all_clients, client_ids: m.client_ids });
const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

// Edits the whole member list locally and saves it in one request (FSD §15.2: one by one or in bulk).
// A save the server refuses marks the rows it names, such as an email no user has.
// Members are picked from the people who share a project with the admin, or
// added by exact email (MSL-21); leaving with unsaved changes asks first.
export default function ProjectMembers({
  projectKey,
  members,
  clients,
  people,
}: {
  projectKey: string;
  members: Member[];
  clients: Client[];
  people: components["schemas"]["Person"][];
}) {
  const t = useTranslations("settings");
  const problemText = useProblemText();
  const [rows, setRows] = useState<Row[]>(() => members.map(toRow));
  const [saved, setSaved] = useState(rows);
  const [other, setOther] = useState(false); // "Someone else": type the email
  const dirty = JSON.stringify(rows) !== JSON.stringify(saved);
  const [status, setStatus] = useState("");
  const [rowErrors, setRowErrors] = useState<Record<number, { text: string; unknownUser: boolean }>>({}); // by row, from the last save

  // Any edit moves or changes rows, so the last save's row errors no longer apply.
  const edit = (next: (rs: Row[]) => Row[]) => {
    setRows(next);
    setRowErrors({});
    setStatus("");
  };
  const update = (i: number, patch: Partial<Row>) => edit((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)));

  useEffect(() => {
    if (!dirty) return;
    const onUnload = (e: BeforeUnloadEvent) => e.preventDefault();
    // A link inside the app navigates without unloading, so clicks ask too.
    const onClick = (e: MouseEvent) => {
      const a = (e.target as Element).closest?.("a[href]");
      if (!a || a.getAttribute("target") === "_blank" || a.hasAttribute("download") || e.metaKey || e.ctrlKey || e.shiftKey) return;
      if (!window.confirm(t("unsavedLeave"))) {
        e.preventDefault();
        e.stopPropagation();
      }
    };
    window.addEventListener("beforeunload", onUnload);
    document.addEventListener("click", onClick, true);
    return () => {
      window.removeEventListener("beforeunload", onUnload);
      document.removeEventListener("click", onClick, true);
    };
  }, [dirty, t]);

  function add(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = new FormData(e.currentTarget);
    const pick = String(form.get("pick") ?? "");
    const email = (pick === "other" ? String(form.get("email") ?? "") : pick).trim();
    if (!email) return;
    if (rows.some((r) => same(r.email, email))) return setStatus(t("alreadyListed"));
    const person = people.find((p) => same(p.email, email));
    edit((rs) => [...rs, { email: person?.email ?? email, name: person?.name ?? email, role: "member", all_clients: true, client_ids: [] }]);
    setOther(false);
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
    if (error) {
      const byRow: typeof rowErrors = {};
      for (const f of error.errors ?? []) {
        const at = /^members\[(\d+)\]\./.exec(f.field);
        if (at) byRow[Number(at[1])] = { text: problemText({ ...error, errors: [f] }), unknownUser: f.code === "unknown_user" };
      }
      setRowErrors(byRow);
      return setStatus(problemText(error));
    }
    const next = data.items.map(toRow);
    edit(() => next);
    setSaved(next);
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
                        {rowErrors[i] && (
                          <span role="alert" className="block max-w-80 text-xs text-danger">
                            {rowErrors[i].text}
                            {rowErrors[i].unknownUser && <span className="block text-muted">{t("unknownUserHint")}</span>}
                          </span>
                        )}
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
                    <button type="button" className={button.quiet} onClick={() => edit((rs) => rs.filter((_, j) => j !== i))}>
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
          {t("person")}
          <select
            name="pick"
            required
            defaultValue=""
            onChange={(e) => setOther(e.target.value === "other")}
            aria-describedby="members-not-listed"
            className={cx(field.input, "w-80")}
          >
            <option value="" disabled>{t("pickPerson")}</option>
            {people
              .filter((p) => !rows.some((r) => same(r.email, p.email)))
              .map((p) => (
                <option key={p.id} value={p.email}>{p.name} · {p.email}</option>
              ))}
            <option value="other">{t("someoneElse")}</option>
          </select>
        </label>
        {other && (
          <label className={field.label}>
            {t("email")}
            <input name="email" type="email" required autoFocus className={cx(field.input, "w-72")} />
          </label>
        )}
        <button className={cx(button.secondary, "h-[34px]")}>{t("addMember")}</button>
        <p id="members-not-listed" className={cx(field.hint, "basis-full")}>{t("notListed")}</p>
      </form>
      <div className="flex items-center gap-3 border-t border-line-soft pt-3">
        <button type="button" onClick={save} className={button.primary}>{t("saveMembers")}</button>
        {status ? (
          <p role="status" className="text-[13px] text-muted">{status}</p>
        ) : (
          dirty && <p className="text-[13px] text-warn">{t("unsaved")}</p>
        )}
      </div>
    </section>
  );
}
