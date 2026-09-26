import Link from "next/link";
import { itemHref } from "@/lib/ask";
import { notFound, redirect } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { day, utc } from "@/lib/format";
import { getMe, serverApi } from "@/lib/server-api";
import { chip, cx, panel, sectionTitle, table } from "@/lib/ui";

// One Ask log entry (FSD §15.4): the exact scope, evidence with scores,
// answer and dropped claims, for debugging trust issues.
export default async function AskLogEntryPage({ params }: { params: Promise<{ id: string }> }) {
  const me = await getMe();
  if (!me) redirect("/login");
  const id = Number((await params).id);
  if (!me.is_admin || !Number.isInteger(id)) notFound();
  const { data: q } = await (await serverApi()).GET("/admin/ask-log/{id}", { params: { path: { id } } });
  if (!q) notFound();
  const t = await getTranslations("askLog");
  const locale = await getLocale();
  const facts: [string, React.ReactNode][] = [
    [t("user"), q.user.name],
    [t("when"), utc(q.created_at, locale)],
    [t("status"), t(`statuses.${q.status}`)],
    [t("feedback"), q.feedback ? `${q.feedback.rating === "up" ? "👍" : "👎"} ${(q.feedback.reasons ?? []).map((r) => t(`reasons.${r}`)).join(", ")}${q.feedback.comment ? ` · “${q.feedback.comment}”` : ""}` : "—"],
    [t("llmCalled"), q.llm_called ? t("yes") : t("no")],
    [t("model"), q.model ?? "—"],
    [t("language"), q.language],
    [t("latency"), q.latency_ms != null ? t("seconds", { s: (q.latency_ms / 1000).toFixed(1) }) : "—"],
    [t("firstClaim"), q.first_claim_ms != null ? t("seconds", { s: (q.first_claim_ms / 1000).toFixed(1) }) : "—"],
  ];
  return (
    <>
      <PageBar>
        <nav className="flex items-center gap-1.5 text-[13px] text-muted">
          <Link href="/admin/ask-log">{t("title")}</Link>
          <span>›</span>
          <span className="text-ink">#{q.id}</span>
        </nav>
      </PageBar>
      <main className="flex max-w-5xl flex-col gap-4 p-4 md:p-5">
        <h1 className="text-lg font-semibold">{q.question}</h1>
        <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5 text-[13px] md:grid-cols-4">
          {facts.map(([k, v]) => (
            <div key={k}>
              <dt className="text-xs text-muted">{k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
        </dl>
        <section className="flex flex-col gap-2">
          <h2 className={sectionTitle}>{t("answer")}</h2>
          {q.claims.length === 0 ? (
            <p className="text-[13px] text-muted">{t("noClaims")}</p>
          ) : (
            <ul className={cx(panel, "flex flex-col gap-1.5 p-3 text-sm")}>
              {q.claims.map((c, i) => (
                <li key={i}>
                  {c.text}{" "}
                  {c.cites.map((k) => (
                    <span key={k} className={cx(chip, "mr-1 bg-accent-soft font-mono text-accent-strong")}>{k}</span>
                  ))}
                </li>
              ))}
            </ul>
          )}
        </section>
        <section className="flex flex-col gap-2">
          <h2 className={sectionTitle}>{t("evidence", { count: q.evidence.length })}</h2>
          {q.evidence.length > 0 && (
            <div className={table.wrap}>
              <table className={table.table}>
                <thead className={table.head}>
                  <tr>
                    <th className={table.th}>{t("key")}</th>
                    <th className={table.th}>{t("ticketTitle")}</th>
                    <th className={table.th}>{t("client")}</th>
                    <th className={table.th}>{t("date")}</th>
                    <th className={table.th}>{t("score")}</th>
                  </tr>
                </thead>
                <tbody>
                  {q.evidence.map((e) => (
                    <tr key={e.key} className={table.row}>
                      <td className={cx(table.td, "font-mono text-xs font-semibold")}>
                        <Link href={itemHref(e.key)}>{e.key}</Link>
                      </td>
                      <td className={table.td}>{e.title}</td>
                      <td className={table.td}>{e.client ?? "—"}</td>
                      <td className={cx(table.td, "whitespace-nowrap")}>{day(e.date, locale)}</td>
                      <td className={cx(table.td, "font-mono text-xs")}>{e.score != null ? e.score.toFixed(4) : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
        <section className="flex flex-col gap-2">
          <h2 className={sectionTitle}>{t("scope")}</h2>
          <pre className={cx(panel, "overflow-x-auto p-3 font-mono text-xs")}>{JSON.stringify(q.scope, null, 2)}</pre>
        </section>
        {q.dropped && (
          <section className="flex flex-col gap-2">
            <h2 className={sectionTitle}>{t("dropped")}</h2>
            <pre className={cx(panel, "overflow-x-auto p-3 font-mono text-xs")}>{JSON.stringify(q.dropped, null, 2)}</pre>
          </section>
        )}
      </main>
    </>
  );
}
