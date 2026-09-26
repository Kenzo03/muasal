import Link from "next/link";
import { redirect } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { utc } from "@/lib/format";
import { getMe, serverApi } from "@/lib/server-api";
import { one } from "@/lib/ticket-query";
import { button, cx, field, table } from "@/lib/ui";

const statuses = ["answered", "not_enough_info", "ai_off", "error"] as const;
type Status = (typeof statuses)[number];

// Admin → Ask log (FSD §15.4): every question, newest first, with quick
// filters for thumbs-down, the unanswered and the slow; an entry opens to its evidence.
export default async function AskLogPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("askLog");
  const locale = await getLocale();
  const v = one(await searchParams);
  const status = statuses.includes(v.status as Status) ? (v.status as Status) : undefined;
  const slow = v.slow === "true";
  const down = v.down === "true";
  const { data } = me.is_admin
    ? await (await serverApi()).GET("/admin/ask-log", {
        params: { query: { status, slow: slow || undefined, down: down || undefined, before: Number(v.before) || undefined } },
      })
    : { data: undefined };
  const filters = { ...(status ? { status } : {}), ...(slow ? { slow: "true" } : {}), ...(down ? { down: "true" } : {}) };
  const quick: [string, Record<string, string>][] = [
    [t("all"), {}],
    [t("thumbsDown"), { down: "true" }],
    [t("notEnough"), { status: "not_enough_info" }],
    [t("slow"), { slow: "true" }],
  ];
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
      </PageBar>
      <main className="flex flex-col gap-3 p-4 md:p-5">
        {!data ? (
          <p className="text-muted">{t("adminsOnly")}</p>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <nav aria-label={t("quick")} className="flex gap-1">
                {quick.map(([label, q]) => {
                  const current = JSON.stringify(q) === JSON.stringify(filters);
                  return (
                    <Link key={label} href={`?${new URLSearchParams(q)}`} aria-current={current ? "page" : undefined} className={cx(button.secondary, current && "border-accent font-semibold")}>
                      {label}
                    </Link>
                  );
                })}
              </nav>
              <form className="ml-auto flex items-center gap-2">
                <label className="flex items-center gap-1.5 text-xs text-muted">
                  {t("status")}
                  <select name="status" defaultValue={status ?? ""} className={field.compact}>
                    <option value="">{t("anyStatus")}</option>
                    {statuses.map((s) => (
                      <option key={s} value={s}>{t(`statuses.${s}`)}</option>
                    ))}
                  </select>
                </label>
                <button type="submit" className={button.secondary}>{t("filter")}</button>
              </form>
            </div>
            {data.items.length === 0 ? (
              <p className="text-muted">{t("empty")}</p>
            ) : (
              <div className={table.wrap}>
                <table className={table.table}>
                  <thead className={table.head}>
                    <tr>
                      <th className={table.th}>{t("when")}</th>
                      <th className={table.th}>{t("user")}</th>
                      <th className={table.th}>{t("question")}</th>
                      <th className={table.th}>{t("status")}</th>
                      <th className={table.th}>{t("citations")}</th>
                      <th className={table.th}>{t("feedback")}</th>
                      <th className={table.th}>{t("latency")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.items.map((q) => (
                      <tr key={q.id} className={table.row}>
                        <td className={cx(table.td, "whitespace-nowrap text-muted")}>{utc(q.created_at, locale)}</td>
                        <td className={table.td}>{q.user.name}</td>
                        <td className={table.td}>
                          <Link href={`/admin/ask-log/${q.id}`}>{q.question}</Link>
                        </td>
                        <td className={cx(table.td, "whitespace-nowrap")}>{t(`statuses.${q.status}`)}</td>
                        <td className={table.td}>{q.citations}</td>
                        <td className={table.td} title={q.feedback?.comment ?? undefined}>
                          {q.feedback ? `${q.feedback.rating === "up" ? "👍" : "👎"} ${(q.feedback.reasons ?? []).map((r) => t(`reasons.${r}`)).join(", ")}` : "—"}
                        </td>
                        <td className={cx(table.td, "whitespace-nowrap", (q.latency_ms ?? 0) > 30000 && "font-semibold text-danger")}>
                          {q.latency_ms != null ? t("seconds", { s: (q.latency_ms / 1000).toFixed(1) }) : "—"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {data.next_before && (
              <Link href={`?${new URLSearchParams({ ...filters, before: String(data.next_before) })}`} className="self-start">{t("older")}</Link>
            )}
          </>
        )}
      </main>
    </>
  );
}
