import Link from "next/link";
import { redirect } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip, StatusDot } from "@/components/Chips";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import { day } from "@/lib/format";
import { getProjects, serverApi } from "@/lib/server-api";
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
  const example = (await getProjects())[0]?.key; // MSL-19: a key from the user's own projects
  const tm = await getTranslations("modules");
  const locale = await getLocale();
  const api = await serverApi();
  const res = query.length >= 2 ? (await api.GET("/search", { params: { query: { q: query } } })).data : undefined;
  const tickets = res?.tickets ?? [];
  const nodes = res?.nodes ?? [];
  const notes = res?.notes ?? [];
  const sections = res?.sections ?? []; // MSL-15: words in documents too
  if (notes[0]?.key === query.toUpperCase() && tickets[0]?.key !== query.toUpperCase()) redirect(`/notes/${notes[0].key}`);
  if (tickets[0]?.key === query.toUpperCase()) redirect(`/t/${tickets[0].key}`);
  const shows = (k: string) => !kind || kind === k;
  const [showNodes, showTickets, showNotes, showSections] = [shows("nodes"), shows("tickets"), shows("notes"), shows("documents")];
  const total = nodes.length + tickets.length + notes.length + sections.length;
  const kinds: [string | undefined, string, number][] = [
    [undefined, t("all"), total],
    ["nodes", t("nodes"), nodes.length],
    ["tickets", t("tickets"), tickets.length],
    ["notes", t("notes"), notes.length],
    ["documents", t("documents"), sections.length],
  ];

  // Each result is a row of a list card; the tile on its left says what it is.
  const row = "flex items-center gap-3 rounded-xl px-3 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink";
  const tile = "flex size-9 shrink-0 items-center justify-center rounded-[10px] bg-well text-ink-soft";
  const project = cx(chip, "bg-well text-ink-soft");
  return (
    <>
      <PageBar>
        <div className="flex flex-col gap-0.5">
          <h1>{query ? t("title", { q: query }) : t("heading")}</h1>
          {example && <p className="text-[13px] text-muted">{t("hint", { example: `${example}-12` })}</p>}
        </div>
      </PageBar>
      <main className="flex max-w-[1040px] flex-col gap-5 px-4 py-4 md:px-5">
        {query.length < 2 || total === 0 ? (
          <div className={cx(panel, "flex flex-col items-center gap-2.5 px-6 py-12 text-center")}>
            <span className="flex size-11 items-center justify-center rounded-full bg-well text-muted">
              <Icon name="search" className="size-5" />
            </span>
            <p className="max-w-sm text-[13.5px] text-ink-soft">{query.length < 2 ? t("short") : t("none", { q: query })}</p>
          </div>
        ) : (
          <>
            <nav aria-label={t("kinds")} className="flex max-w-full gap-0.5 self-start overflow-x-auto rounded-[11px] bg-well p-[3px] text-[13px]">
              {kinds.map(([k, label, count]) => (
                <Link
                  key={label}
                  href={`/search?${new URLSearchParams(k ? { q: query, kind: k } : { q: query })}`}
                  aria-current={kind === k ? "page" : undefined}
                  className={cx(
                    "flex h-[30px] shrink-0 items-center gap-1.5 rounded-lg px-3 no-underline",
                    kind === k ? "bg-white font-bold text-ink shadow-[0_1px_2px_rgba(43,36,32,0.1)] hover:text-ink" : "font-semibold text-ink-soft hover:text-ink",
                  )}
                >
                  {label}
                  <span className="text-[11.5px] font-semibold tabular-nums text-muted">{count}</span>
                </Link>
              ))}
            </nav>
            {showNodes && nodes.length > 0 && (
              <section aria-labelledby="nodes-title" className="flex flex-col gap-2">
                <h2 id="nodes-title" className={sectionTitle}>{t("nodes")}</h2>
                <ul className={cx(panel, "flex flex-col gap-0.5 p-1.5")}>
                  {nodes.map((n) => (
                    <li key={n.id}>
                      <Link href={`/p/${n.project_key}/modules/${n.id}`} className={row}>
                        <span className={tile}>
                          <Icon name={n.type === "menu" ? "screen" : "folder"} />
                        </span>
                        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                          <span className="text-sm font-semibold"><Highlight text={n.path.join(" › ")} q={query} /></span>
                          <span className="text-xs text-muted">
                            {tm(n.type)}
                            {n.aliases.length > 0 && <> · <Highlight text={t("aliases", { aliases: n.aliases.join(", ") })} q={query} /></>}
                            {n.code && <> · <Highlight text={n.code} q={query} /></>}
                          </span>
                        </span>
                        <span className={project}>{n.project_key}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </section>
            )}
            {showTickets && tickets.length > 0 && (
              <section aria-labelledby="tickets-title" className="flex flex-col gap-2">
                <h2 id="tickets-title" className={sectionTitle}>{t("tickets")}</h2>
                <ul className={cx(panel, "flex flex-col gap-0.5 p-1.5")}>
                  {tickets.map((it) => (
                    <li key={it.key}>
                      <Link href={`/t/${it.key}`} className={row}>
                        <span className={tile}>
                          <StatusDot color={it.status.color} className="size-2.5" />
                        </span>
                        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                          <span className="text-sm font-semibold"><Highlight text={it.title} q={query} /></span>
                          <span className="text-xs text-muted">
                            <span className="font-bold">{it.key}</span> · {it.status.name}
                          </span>
                        </span>
                        <ClientChip client={it.client} coreLabel={t("core")} />
                        <span className={project}>{it.project_key}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </section>
            )}
            {showNotes && notes.length > 0 && (
              <section aria-labelledby="notes-title" className="flex flex-col gap-2">
                <h2 id="notes-title" className={sectionTitle}>{t("notes")}</h2>
                <ul className={cx(panel, "flex flex-col gap-0.5 p-1.5")}>
                  {notes.map((n) => (
                    <li key={n.key}>
                      <Link href={`/notes/${n.key}`} className={row}>
                        <span className={tile}>
                          <Icon name="notes" />
                        </span>
                        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                          <span className="text-sm font-semibold"><Highlight text={n.title} q={query} /></span>
                          <span className="text-xs text-muted">
                            <span className="font-bold">{n.key}</span> · {day(n.decided_on, locale)}
                          </span>
                        </span>
                        <span className={project}>{n.project_key}</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </section>
            )}
            {showSections && sections.length > 0 && (
              <section aria-labelledby="documents-title" className="flex flex-col gap-2">
                <h2 id="documents-title" className={sectionTitle}>{t("documents")}</h2>
                <ul className={cx(panel, "flex flex-col gap-0.5 p-1.5")}>
                  {sections.map((sec) => (
                    <li key={`${sec.document_key}/${sec.number}`}>
                      <Link href={`/documents/${sec.document_key}#s-${sec.number}`} className={row}>
                        <span className={tile}>
                          <Icon name="file" />
                        </span>
                        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                          <span className="text-sm font-semibold">
                            <Highlight text={sec.number.startsWith("s") ? sec.title : `${sec.number} ${sec.title}`} q={query} />
                          </span>
                          <span className="text-xs text-muted">
                            <span className="font-bold">{sec.document_key}</span> · {sec.document_title}
                            {sec.superseded && <> · {t("superseded")}</>}
                          </span>
                          <span className="text-xs text-ink-soft"><Highlight text={sec.excerpt} q={query} /></span>
                        </span>
                        <span className={project}>{sec.project_key}</span>
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
