import { Fragment } from "react";
import Link from "next/link";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip, StatusDot, showsClients } from "@/components/Chips";
import Icon from "@/components/Icon";
import { day } from "@/lib/format";
import type { Client, TimelineEntry, TimelineNote } from "@/lib/problem";
import { button, chip, cx, field } from "@/lib/ui";

const types = ["bug", "change_request", "feature"] as const;

// The rail beside an entry: a ring while the ticket is open, a dot once closed.
function Rail({ color, open }: { color: string; open?: boolean }) {
  return (
    <div className="flex flex-col items-center">
      <span
        className={open ? "mt-[17px] size-3 shrink-0 rounded-full border-[2.5px] bg-ground" : "mt-5 size-3 shrink-0 rounded-full ring-4 ring-ground"}
        style={open ? { borderColor: color } : { background: color }}
      />
      <span className="w-0.5 grow bg-line" />
    </div>
  );
}

// The Timeline tab (FSD §7.4, story 1): open tickets pinned under "In progress",
// then closed ones newest first by close date, each with what changed and why.
// Decision notes sit among them by decision date (§9.4); while more tickets
// wait to load, notes older than the last loaded one wait too.
// The filters are a plain GET form, so the URL holds them.
export default async function Timeline({ items, notes, failed, more, limit, clients, values }: {
  items: TimelineEntry[];
  notes: TimelineNote[];
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
  const closedTickets = items.filter((it) => it.closed_at);
  const oldest = closedTickets.at(-1)?.closed_at?.slice(0, 10) ?? "";
  const shownNotes = more ? notes.filter((n) => n.decided_on >= oldest) : notes;
  type Row = { kind: "ticket"; at: string; it: TimelineEntry } | { kind: "note"; at: string; note: TimelineNote };
  const closed: Row[] = [
    ...closedTickets.map((it): Row => ({ kind: "ticket", at: it.closed_at!, it })),
    ...shownNotes.map((note): Row => ({ kind: "note", at: note.decided_on, note })),
  ].sort((a, b) => (a.at.slice(0, 10) === b.at.slice(0, 10) ? 0 : a.at < b.at ? 1 : -1));
  const withClients = showsClients(clients);
  const requester = (it: TimelineEntry) => `${it.requester.name}${it.requester.title ? ` (${it.requester.title})` : ""}`;
  const grid = "grid grid-cols-[88px_22px_minmax(0,1fr)] gap-x-3 md:grid-cols-[112px_24px_minmax(0,1fr)] md:gap-x-3.5";
  // Each filter is a chip: its name, then a borderless control.
  const pick = "flex h-9 items-center gap-1 rounded-[10px] border border-line bg-white pl-3 pr-1 text-[13.5px] font-semibold text-ink focus-within:border-accent";
  const control = "h-full cursor-pointer rounded-[10px] bg-transparent pr-1 text-[13.5px] font-medium text-muted outline-none";
  const card = "rounded-2xl border border-line bg-white shadow-[0_1px_2px_rgba(43,36,32,0.04)]";

  return (
    <>
      <form aria-label={t("filters")} className="flex flex-wrap items-center gap-2">
        {withClients && (
          <label className={pick}>
            {t("client")}
            <select name="client" defaultValue={values.client ?? ""} className={control}>
              <option value="">{t("allClients")}</option>
              <option value="core">{t("core")}</option>
              {clients.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          </label>
        )}
        <label className={pick}>
          {t("type")}
          <select name="type" defaultValue={values.type ?? ""} className={control}>
            <option value="">{t("allTypes")}</option>
            {types.map((ty) => (
              <option key={ty} value={ty}>{tt(ty)}</option>
            ))}
          </select>
        </label>
        <label className={pick}>
          {t("from")}
          <input type="date" name="from" defaultValue={values.from} className={control} />
        </label>
        <label className={pick}>
          {t("to")}
          <input type="date" name="to" defaultValue={values.to} className={control} />
        </label>
        {/* Checked sends sub=1 before the hidden sub=0, and the page reads the first value; unchecked sends sub=0. */}
        <label className="flex h-9 items-center gap-2 px-1 text-[13.5px] font-semibold text-ink">
          <input type="checkbox" name="sub" value="1" defaultChecked={values.sub !== "0"} className="size-4 accent-accent" />
          {t("subNodes")}
        </label>
        <input type="hidden" name="sub" value="0" />
        <button className={button.secondary}>{t("apply")}</button>
      </form>
      {failed ? (
        <p role="alert" className={field.error}>{t("filterError")}</p>
      ) : items.length === 0 && notes.length === 0 ? (
        <p className="text-muted">{t("empty")}</p>
      ) : (
        <>
          {open.length > 0 && (
            <section aria-labelledby="open-title" className="flex flex-col gap-2.5">
              <h2 id="open-title" className="text-[13px] font-extrabold text-accent">{t("open", { count: open.length })}</h2>
              <ol className="flex flex-col">
                {open.map((it) => (
                  <li key={it.key} className={grid}>
                    <div className="pt-3.5 text-right text-xs font-semibold text-muted">{t("createdOn", { date: day(it.created_at, locale) })}</div>
                    <Rail color={it.status.color} open />
                    <article
                      aria-label={`${it.key} ${it.title}`}
                      className={cx(card, "mb-3 flex flex-wrap items-center gap-x-2.5 gap-y-1 px-4 py-3 text-[13px]")}
                    >
                      <Link href={`/t/${it.key}`} className="font-bold text-muted no-underline hover:text-ink">{it.key}</Link>
                      <Link href={`/t/${it.key}`} className="text-[14.5px] font-bold text-ink no-underline hover:text-ink hover:underline">{it.title}</Link>
                      {withClients && <ClientChip client={it.client} coreLabel={t("core")} />}
                      <span className="text-muted">{requester(it)}</span>
                      <span className="ml-auto flex items-center gap-1.5 font-semibold text-ink-soft">
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
            <section aria-labelledby="closed-title" className="flex flex-col gap-2.5">
              <h2 id="closed-title" className="text-[13px] font-extrabold text-ink-soft">{t("closed", { count: closed.length })}</h2>
              <ol className="flex flex-col">
                {closed.map((row, i) => {
                  // A year marker where the year changes; the dates keep their year for screen readers.
                  const year = row.at.slice(0, 4);
                  const marker = i === 0 || closed[i - 1].at.slice(0, 4) !== year ? (
                    <li aria-hidden="true" className={cx(grid, "pb-2 pt-1")}>
                      <span className="text-right text-lg font-extrabold tracking-[-0.02em] text-ink">{year}</span>
                      <span className={cx("mx-auto w-0.5 bg-line", i === 0 && "opacity-0")} />
                    </li>
                  ) : null;
                  if (row.kind === "note") {
                    const n = row.note;
                    return (
                      <Fragment key={n.key}>
                      {marker}
                      <li className={grid}>
                        <div className="flex flex-col items-end gap-0.5 pt-4">
                          <span className="text-sm font-extrabold">{day(n.decided_on, locale)}</span>
                          <span className="text-xs text-muted">{t("note")}</span>
                        </div>
                        <Rail color="#8A7F76" />
                        <article aria-label={`${n.key} ${n.title}`} className="mb-3 flex flex-col gap-2 rounded-2xl border border-dashed border-field bg-paper px-5 py-4">
                          <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
                            <Link href={`/notes/${n.key}`} className="text-[13px] font-bold no-underline">{n.key}</Link>
                            <span className={cx(chip, "rounded-full bg-well text-ink-soft")}>{t("note")}</span>
                            {withClients && <ClientChip client={n.client} coreLabel={t("core")} />}
                            {n.attendees && <span>{t("attendees", { names: n.attendees })}</span>}
                          </div>
                          <Link href={`/notes/${n.key}`} className="text-base font-extrabold text-ink no-underline hover:text-ink hover:underline">
                            {n.title}
                          </Link>
                          <p className="line-clamp-3 whitespace-pre-wrap text-sm leading-relaxed">{n.body}</p>
                        </article>
                      </li>
                      </Fragment>
                    );
                  }
                  const it = row.it;
                  const implemented = it.decision?.outcome === "implemented";
                  return (
                    <Fragment key={it.key}>
                    {marker}
                    <li className={grid}>
                      <div className="flex flex-col items-end gap-0.5 pt-4">
                        <span className="text-sm font-extrabold">{day(it.closed_at!, locale)}</span>
                        <span className="text-xs text-muted">{it.status.name}</span>
                      </div>
                      <Rail color={it.status.color} />
                      <article aria-label={`${it.key} ${it.title}`} className={cx(card, "mb-3 flex flex-col gap-3 px-5 py-4")}>
                        <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
                          <Link href={`/t/${it.key}`} className="text-[13px] font-bold text-ink-soft no-underline hover:text-ink">{it.key}</Link>
                          {it.decision && (
                            <span className={cx(chip, "rounded-full", implemented ? "bg-ok-soft text-ok" : "bg-well text-ink-soft")}>
                              {implemented ? t("implemented") : t("rejected")}
                            </span>
                          )}
                          {it.decision?.superseded_by && (
                            <span className={cx(chip, "rounded-full bg-warn-soft text-warn")}>{t("supersededBy", { key: it.decision.superseded_by })}</span>
                          )}
                          {withClients && <ClientChip client={it.client} coreLabel={t("core")} />}
                          <span>{tt(it.type)} · {t("requestedBy", { name: requester(it) })}</span>
                        </div>
                        <Link href={`/t/${it.key}`} className="text-base font-extrabold leading-snug tracking-[-0.01em] text-ink no-underline hover:text-ink hover:underline">
                          {it.title}
                        </Link>
                        {it.decision && (
                          <>
                            <div className="grid gap-x-6 gap-y-3 text-sm leading-relaxed md:grid-cols-2">
                              <div>
                                <h3 className="text-[12.5px] font-bold text-muted">{implemented ? t("whatChanged") : t("whatDecided")}</h3>
                                <p className="mt-1 whitespace-pre-wrap">{it.decision.what_changed}</p>
                              </div>
                              <div>
                                <h3 className="text-[12.5px] font-bold text-muted">{t("why")}</h3>
                                <p className="mt-1 whitespace-pre-wrap">{it.decision.why}</p>
                              </div>
                            </div>
                            {it.decision.alternatives && (
                              <details className="group rounded-xl bg-paper px-4 py-2.5 text-[13px]">
                                <summary className="flex cursor-pointer items-center gap-1.5 font-bold text-link">
                                  <Icon name="chevronRight" className="size-3.5 transition-transform group-open:rotate-90" />
                                  {t("alternatives")}
                                </summary>
                                <p className="mt-1.5 whitespace-pre-wrap text-sm leading-relaxed text-ink">{it.decision.alternatives}</p>
                              </details>
                            )}
                          </>
                        )}
                      </article>
                    </li>
                    </Fragment>
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
