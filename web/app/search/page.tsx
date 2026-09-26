import Link from "next/link";
import { redirect } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip, StatusDot } from "@/components/Chips";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import { day } from "@/lib/format";
import { serverApi } from "@/lib/server-api";
import { one } from "@/lib/ticket-query";
import { chip, cx, panel, sectionTitle } from "@/lib/ui";

// Marks the first place q occurs in text, ignoring case.
function Highlight({ text, q }: { text: string; q: string }) {
  const at = text.toLowerCase().indexOf(q.toLowerCase());
  if (q === "" || at < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, at)}
      <mark className="rounded-sm bg-[#FBE3C8] px-px text-inherit">{text.slice(at, at + q.length)}</mark>
      {text.slice(at + q.length)}
    </>
  );
}

// Search (FSD §6.1–6.2): the tickets, menus and decision notes the user may open, across their
// projects. A query that is a visible ticket's key opens that ticket.
export default async function SearchPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { q = "", kind } = one(await searchParams);
  const query = q.trim();
  const t = await getTranslations("search");
  const tm = await getTranslations("modules");
  const locale = await getLocale();
  const api = await serverApi();
  const res = query.length >= 2 ? (await api.GET("/search", { params: { query: { q: query } } })).data : undefined;
  const tickets = res?.tickets ?? [];
  const nodes = res?.nodes ?? [];
  const notes = res?.notes ?? [];
  if (notes[0]?.key === query.toUpperCase() && tickets[0]?.key !== query.toUpperCase()) redirect(`/notes/${notes[0].key}`);
  if (tickets[0]?.key === query.toUpperCase()) redirect(`/t/${tickets[0].key}`);
  const showNodes = kind !== "tickets" && kind !== "notes";
  const showTickets = kind !== "nodes" && kind !== "notes";
  const showNotes = kind !== "nodes" && kind !== "tickets";
  const kinds: [string | undefined, string, number][] = [
    [undefined, t("all"), nodes.length + tickets.length + notes.length],
    ["nodes", t("nodes"), nodes.length],
    ["tickets", t("tickets"), tickets.length],
    ["notes", t("notes"), notes.length],
  ];

  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{query ? t("title", { q: query }) : t("heading")}</h1>
        <p className="text-[13px] text-muted">{t("hint")}</p>
      </PageBar>
      <main className="flex max-w-[1040px] flex-col gap-4 px-4 py-4 md:px-5">
        {query.length < 2 ? (
          <p className="text-muted">{t("short")}</p>
        ) : nodes.length + tickets.length + notes.length === 0 ? (
          <p className="text-muted">{t("none", { q: query })}</p>
        ) : (
          <>
            <nav aria-label={t("kinds")} className="flex gap-1 border-b border-line">
              {kinds.map(([k, label, count]) => (
                <Link
                  key={label}
                  href={`/search?${new URLSearchParams(k ? { q: query, kind: k } : { q: query })}`}
                  aria-current={kind === k ? "page" : undefined}
                  className={cx(
                    "flex items-center gap-1.5 border-b-2 px-3 py-2 text-[13px] no-underline",
                    kind === k ? "border-accent font-semibold text-ink hover:text-ink" : "border-transparent text-muted hover:text-ink",
                  )}
                >
                  {label}
                  <span className="font-mono text-[11px]">{count}</span>
                </Link>
              ))}
            </nav>
            {showNodes && nodes.length > 0 && (
              <section aria-labelledby="nodes-title" className="flex flex-col gap-2">
                <h2 id="nodes-title" className={sectionTitle}>{t("nodes")}</h2>
                <ul className={cx(panel, "divide-y divide-line-soft")}>
                  {nodes.map((n) => (
                    <li key={n.id}>
                      <Link
                        href={`/p/${n.project_key}/modules/${n.id}`}
                        className="flex items-center gap-2.5 px-3.5 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink"
                      >
                        <Icon name={n.type === "menu" ? "screen" : "folder"} className="text-muted" />
                        <span className="flex min-w-0 flex-col gap-0.5">
                          <span className="text-sm font-semibold"><Highlight text={n.path.join(" › ")} q={query} /></span>
                          {(n.aliases.length > 0 || n.code) && (
                            <span className="text-xs text-muted">
                              {n.aliases.length > 0 && <Highlight text={t("aliases", { aliases: n.aliases.join(", ") })} q={query} />}
                              {n.aliases.length > 0 && n.code ? " · " : ""}
                              {n.code && <span className="font-mono"><Highlight text={n.code} q={query} /></span>}
                            </span>
                          )}
                        </span>
                        <span className="ml-auto text-xs text-muted">{tm(n.type)}</span>
                        <span className={cx(chip, "bg-ground font-mono text-[#4A423C]")}>{n.project_key}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </section>
            )}
            {showTickets && tickets.length > 0 && (
              <section aria-labelledby="tickets-title" className="flex flex-col gap-2">
                <h2 id="tickets-title" className={sectionTitle}>{t("tickets")}</h2>
                <ul className={cx(panel, "divide-y divide-line-soft")}>
                  {tickets.map((it) => (
                    <li key={it.key}>
                      <Link
                        href={`/t/${it.key}`}
                        className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3.5 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink"
                      >
                        <span className="w-24 font-mono text-[13px] font-semibold text-link">{it.key}</span>
                        <span className="min-w-0 flex-1 text-sm font-semibold"><Highlight text={it.title} q={query} /></span>
                        <span className="flex items-center gap-1.5 text-xs">
                          <StatusDot color={it.status.color} />
                          {it.status.name}
                        </span>
                        <ClientChip client={it.client} coreLabel={t("core")} />
                        <span className={cx(chip, "bg-ground font-mono text-[#4A423C]")}>{it.project_key}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </section>
            )}
            {showNotes && notes.length > 0 && (
              <section aria-labelledby="notes-title" className="flex flex-col gap-2">
                <h2 id="notes-title" className={sectionTitle}>{t("notes")}</h2>
                <ul className={cx(panel, "divide-y divide-line-soft")}>
                  {notes.map((n) => (
                    <li key={n.key}>
                      <Link
                        href={`/notes/${n.key}`}
                        className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3.5 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink"
                      >
                        <span className="w-24 font-mono text-[13px] font-semibold text-link">{n.key}</span>
                        <span className="min-w-0 flex-1 text-sm font-semibold"><Highlight text={n.title} q={query} /></span>
                        <span className="text-xs text-muted">{day(n.decided_on, locale)}</span>
                        <span className={cx(chip, "bg-ground font-mono text-[#4A423C]")}>{n.project_key}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </section>
            )}
          </>
        )}
      </main>
    </>
  );
}
