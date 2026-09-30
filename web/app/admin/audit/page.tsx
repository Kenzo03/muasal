import Link from "next/link";
import { redirect } from "next/navigation";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { changeLines } from "@/lib/activity";
import { dateTime } from "@/lib/format";
import { getMe, serverApi } from "@/lib/server-api";
import { one } from "@/lib/ticket-query";
import { button, cx, field, table } from "@/lib/ui";

const entities = ["ticket", "node", "project", "client", "contact", "user", "note", "document", "repo", "token", "ai_settings", "summary_schedule", "backup", "import"];
// Where a subject has its own page (MSL-27).
const pages: Record<string, string> = { ticket: "/t/", note: "/notes/", document: "/documents/" };

// Admin → Audit log (FSD §15.4): read-only, filtered by actor, entity, action
// and date, with a CSV export of the same filter.
export default async function AuditPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("audit");
  const tf = await getTranslations("activity.fields");
  const entity = (e: string) => (t.has(`entities.${e}`) ? t(`entities.${e}`) : e);
  const fieldName = (k: string) => (tf.has(k) ? tf(k) : k);
  const locale = await getLocale();
  const timeZone = await getTimeZone();
  const v = one(await searchParams);
  const filter = {
    actor_id: Number(v.actor) || undefined,
    entity: v.entity || undefined,
    action: v.action || undefined,
    from: v.from || undefined,
    to: v.to || undefined,
  };
  const api = await serverApi();
  const [page, users] = me.is_admin
    ? await Promise.all([api.GET("/admin/audit", { params: { query: { ...filter, before: Number(v.before) || undefined } } }), api.GET("/admin/users")])
    : [undefined, undefined];
  const kept = Object.fromEntries(Object.entries({ actor: v.actor, entity: v.entity, action: v.action, from: v.from, to: v.to }).filter(([, x]) => x)) as Record<string, string>;
  const exportQuery = new URLSearchParams(Object.entries(filter).filter(([, x]) => x !== undefined).map(([k, x]) => [k, String(x)]));
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
        {page?.data && (
          <a href={`/api/v1/admin/audit/export?${exportQuery}`} className={cx(button.secondary, "ml-auto")} download>
            {t("export")}
          </a>
        )}
      </PageBar>
      <main className="flex flex-col gap-3 p-4 md:p-5">
        {!page?.data ? (
          <p className="text-muted">{t("adminsOnly")}</p>
        ) : (
          <>
            <form aria-label={t("filters")} className="flex flex-wrap items-end gap-2">
              <label className={field.label}>
                {t("actor")}
                <select name="actor" defaultValue={v.actor ?? ""} className={field.compact}>
                  <option value="">{t("anyone")}</option>
                  {(users?.data?.items ?? []).map((u) => (
                    <option key={u.id} value={u.id}>{u.name}</option>
                  ))}
                </select>
              </label>
              <label className={field.label}>
                {t("entity")}
                <select name="entity" defaultValue={v.entity ?? ""} className={field.compact}>
                  <option value="">{t("anything")}</option>
                  {entities.map((e) => (
                    <option key={e} value={e}>{entity(e)}</option>
                  ))}
                </select>
              </label>
              <label className={field.label}>
                {t("action")}
                <input name="action" defaultValue={v.action ?? ""} placeholder="create" className={cx(field.compact, "w-32")} />
              </label>
              <label className={field.label}>
                {t("from")}
                <input type="date" name="from" defaultValue={v.from ?? ""} className={field.compact} />
              </label>
              <label className={field.label}>
                {t("to")}
                <input type="date" name="to" defaultValue={v.to ?? ""} className={field.compact} />
              </label>
              <button type="submit" className={button.secondary}>{t("filter")}</button>
              {Object.keys(kept).length > 0 && <Link href="?" className="self-center text-[13px]">{t("clear")}</Link>}
            </form>
            {page.data.items.length === 0 ? (
              <p className="text-muted">{t("empty")}</p>
            ) : (
              <div className={table.wrap}>
                <table className={table.table}>
                  <thead className={table.head}>
                    <tr>
                      <th className={table.th}>{t("when")}</th>
                      <th className={table.th}>{t("actor")}</th>
                      <th className={table.th}>{t("via")}</th>
                      <th className={table.th}>{t("entity")}</th>
                      <th className={table.th}>{t("action")}</th>
                      <th className={table.th}>{t("project")}</th>
                      <th className={table.th}>{t("changes")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {page.data.items.map((e) => (
                      <tr key={e.id} className={table.row}>
                        <td className={cx(table.td, "whitespace-nowrap text-muted")}>{dateTime(e.occurred_at, locale, timeZone)}</td>
                        <td className={table.td}>{e.actor?.name ?? t("system")}</td>
                        <td className={cx(table.td, "text-muted")}>
                          {e.token ? t("vias.token", { token: e.token }) : t.has(`vias.${e.via}`) ? t(`vias.${e.via}`) : e.via}
                        </td>
                        <td className={cx(table.td, "whitespace-nowrap")}>
                          {entity(e.entity)}{" "}
                          {e.subject && pages[e.entity] ? (
                            <Link href={pages[e.entity] + e.subject} className="font-semibold">{e.subject}</Link>
                          ) : e.subject ? (
                            <span className="font-semibold">{e.subject}</span>
                          ) : (
                            e.entity_id > 0 && <span className="font-mono text-xs text-muted">#{e.entity_id}</span>
                          )}
                        </td>
                        <td className={table.td}>{e.action}</td>
                        <td className={cx(table.td, "font-mono text-xs")}>{e.project ?? "—"}</td>
                        <td className={cx(table.td, "max-w-md text-xs text-muted")}>
                          <ul className="flex flex-col gap-0.5">
                            {changeLines(e.changes, fieldName).map((line, i) => (
                              <li key={i} className="line-clamp-2 break-words" title={line}>{line}</li>
                            ))}
                          </ul>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {page.data.next_before && (
              <Link href={`?${new URLSearchParams({ ...kept, before: String(page.data.next_before) })}`} className="self-start">{t("older")}</Link>
            )}
          </>
        )}
      </main>
    </>
  );
}
