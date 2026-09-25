import Link from "next/link";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip, StatusDot } from "@/components/Chips";
import { day } from "@/lib/format";
import type { Client, TimelineEntry } from "@/lib/problem";
import { button, chip, cx, field, sectionTitle } from "@/lib/ui";

const types = ["bug", "change_request", "feature"] as const;

// The rail beside an entry: a ring while the ticket is open, a dot once closed.
function Rail({ color, open }: { color: string; open?: boolean }) {
  return (
    <div className="flex flex-col items-center">
      <span
        className={open ? "mt-3.5 size-2 rounded-full border-2 bg-ground" : "mt-[15px] size-3 rounded-full"}
        style={open ? { borderColor: color } : { background: color }}
      />
      <span className="w-0.5 grow bg-line" />
    </div>
  );
}

// The Timeline tab (FSD §7.4, story 1): open tickets pinned under "In progress",
// then closed ones newest first by close date, each with what changed and why.
// The filters are a plain GET form, so the URL holds them.
export default async function Timeline({ items, failed, more, limit, clients, values }: {
  items: TimelineEntry[];
  failed: boolean;
  more: boolean;
  limit: number;
  clients: Client[];
  values: Record<string, string>;
}) {
  const t = await getTranslations("nodePage");
  const tt = await getTranslations("ticketTypes");
  const locale = await getLocale();
  const open = items.filter((it) => !it.closed_at);
  const closed = items.filter((it) => it.closed_at);
  const requester = (it: TimelineEntry) => `${it.requester.name}${it.requester.title ? ` (${it.requester.title})` : ""}`;
  const grid = "grid grid-cols-[88px_22px_minmax(0,1fr)] gap-x-3 md:grid-cols-[140px_22px_minmax(0,1fr)] md:gap-x-3.5";
  const label = "flex flex-col gap-1 text-xs text-muted";

  return (
    <>
      <form aria-label={t("filters")} className="flex flex-wrap items-end gap-2">
        <label className={label}>
          {t("client")}
          <select name="client" defaultValue={values.client ?? ""} className={field.compact}>
            <option value="">{t("allClients")}</option>
            <option value="core">{t("core")}</option>
            {clients.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
        </label>
        <label className={label}>
          {t("type")}
          <select name="type" defaultValue={values.type ?? ""} className={field.compact}>
            <option value="">{t("allTypes")}</option>
            {types.map((ty) => (
              <option key={ty} value={ty}>{tt(ty)}</option>
            ))}
          </select>
        </label>
        <label className={label}>
          {t("from")}
          <input type="date" name="from" defaultValue={values.from} className={field.compact} />
        </label>
        <label className={label}>
          {t("to")}
          <input type="date" name="to" defaultValue={values.to} className={field.compact} />
        </label>
        {/* Checked sends sub=1 before the hidden sub=0, and the page reads the first value; unchecked sends sub=0. */}
        <label className="flex h-8 items-center gap-1.5 text-[13px] text-ink">
          <input type="checkbox" name="sub" value="1" defaultChecked={values.sub !== "0"} className="size-4 accent-accent" />
          {t("subNodes")}
        </label>
        <input type="hidden" name="sub" value="0" />
        <button className={button.secondary}>{t("apply")}</button>
      </form>
      {failed ? (
        <p role="alert" className={field.error}>{t("filterError")}</p>
      ) : items.length === 0 ? (
        <p className="text-muted">{t("empty")}</p>
      ) : (
        <>
          {open.length > 0 && (
            <section aria-labelledby="open-title" className="flex flex-col gap-2">
              <h2 id="open-title" className={sectionTitle}>{t("open", { count: open.length })}</h2>
              <ol className="flex flex-col">
                {open.map((it) => (
                  <li key={it.key} className={grid}>
                    <div className="pt-3 text-right text-xs text-muted">{t("createdOn", { date: day(it.created_at, locale) })}</div>
                    <Rail color={it.status.color} open />
                    <article
                      aria-label={`${it.key} ${it.title}`}
                      className="mb-2 flex flex-wrap items-center gap-x-2.5 gap-y-1 rounded border border-line bg-white px-3.5 py-2.5 text-[13px]"
                    >
                      <Link href={`/t/${it.key}`} className="font-mono font-semibold">{it.key}</Link>
                      <Link href={`/t/${it.key}`} className="font-semibold text-ink no-underline hover:text-ink hover:underline">{it.title}</Link>
                      <ClientChip client={it.client} coreLabel={t("core")} />
                      <span className="text-muted">{requester(it)}</span>
                      <span className="ml-auto flex items-center gap-1.5">
                        <StatusDot color={it.status.color} />
                        {it.status.name}
                      </span>
                    </article>
                  </li>
                ))}
              </ol>
            </section>
          )}
          {closed.length > 0 && (
            <section aria-labelledby="closed-title" className="flex flex-col gap-2">
              <h2 id="closed-title" className={sectionTitle}>{t("closed", { count: closed.length })}</h2>
              <ol className="flex flex-col">
                {closed.map((it) => {
                  const implemented = it.decision?.outcome === "implemented";
                  return (
                    <li key={it.key} className={grid}>
                      <div className="flex flex-col items-end gap-0.5 pt-3">
                        <span className="text-sm font-semibold">{day(it.closed_at!, locale)}</span>
                        <span className="text-xs text-muted">{it.status.name}</span>
                      </div>
                      <Rail color={it.status.color} />
                      <article aria-label={`${it.key} ${it.title}`} className="mb-3 flex flex-col gap-2 rounded border border-line bg-white px-3.5 py-3">
                        <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
                          <Link href={`/t/${it.key}`} className="font-mono text-[13px] font-semibold">{it.key}</Link>
                          {it.decision && (
                            <span className={cx(chip, implemented ? "bg-ok-soft text-ok" : "bg-well text-[#4A423C]")}>
                              {implemented ? t("implemented") : t("rejected")}
                            </span>
                          )}
                          <ClientChip client={it.client} coreLabel={t("core")} />
                          <span>{tt(it.type)} · {t("requestedBy", { name: requester(it) })}</span>
                        </div>
                        <Link href={`/t/${it.key}`} className="text-[15px] font-semibold text-ink no-underline hover:text-ink hover:underline">
                          {it.title}
                        </Link>
                        {it.decision && (
                          <>
                            <div className="grid gap-x-5 gap-y-2 text-[13px] leading-normal md:grid-cols-2">
                              <div>
                                <h3 className="text-[11px] font-semibold uppercase tracking-[0.04em] text-muted">{implemented ? t("whatChanged") : t("whatDecided")}</h3>
                                <p className="mt-0.5 whitespace-pre-wrap">{it.decision.what_changed}</p>
                              </div>
                              <div>
                                <h3 className="text-[11px] font-semibold uppercase tracking-[0.04em] text-muted">{t("why")}</h3>
                                <p className="mt-0.5 whitespace-pre-wrap">{it.decision.why}</p>
                              </div>
                            </div>
                            {it.decision.alternatives && (
                              <details className="text-xs">
                                <summary className="cursor-pointer text-link">{t("alternatives")}</summary>
                                <p className="mt-1.5 whitespace-pre-wrap text-[13px]">{it.decision.alternatives}</p>
                              </details>
                            )}
                          </>
                        )}
                      </article>
                    </li>
                  );
                })}
              </ol>
            </section>
          )}
          {more && (
            <Link href={`?${new URLSearchParams({ ...values, limit: String(limit + 50) })}`} className={cx(button.secondary, "self-start")}>
              {t("loadMore")}
            </Link>
          )}
        </>
      )}
    </>
  );
}
