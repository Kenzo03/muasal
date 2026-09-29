import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import { ClientChip, DraftStatusChip, showsClients } from "@/components/Chips";
import Icon from "@/components/Icon";
import Markdown from "@/components/Markdown";
import PageBar from "@/components/PageBar";
import { asTables } from "@/lib/convert";
import { dateTime, dayOf } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { button, chip, cx, panel, sectionTitle } from "@/lib/ui";
import DraftButton from "./DraftButton";
import Replace from "./Replace";

// A document page (R-MR-15): the Markdown by section, each with an anchor so
// citation chips such as HRIS-DOC1/7.4 open at the right place, the nodes each
// section produced, the original file and the tree drafts.
export default async function DocumentPage({ params }: { params: Promise<{ docKey: string }> }) {
  const { docKey } = await params;
  const api = await serverApi();
  const { data: doc } = await api.GET("/documents/{key}", { params: { path: { key: docKey } } });
  if (!doc) notFound();
  const [project, clients, docs] = await Promise.all([
    getProject(doc.project_key),
    api.GET("/projects/{key}/clients", { params: { path: { key: doc.project_key } } }),
    api.GET("/projects/{key}/documents", { params: { path: { key: doc.project_key } } }),
  ]);
  const t = await getTranslations("documents");
  const tt = await getTranslations("ticket");
  const locale = await getLocale();
  const timeZone = await getTimeZone();
  const admin = project?.role === "admin";
  // Unnumbered headings get a running s1, s2…: those show no number.
  const numbered = (n: string) => !/^s\d/.test(n);
  return (
    <>
      <PageBar>
        <div className="flex min-w-0 flex-1 basis-full flex-col gap-1 md:basis-0">
          <nav aria-label={tt("path")} className="flex items-center gap-1.5 text-[13px] text-muted">
            <Link href={`/p/${doc.project_key}/board`}>{doc.project_key}</Link>
            <Icon name="chevronRight" className="size-3.5" />
            <Link href={`/p/${doc.project_key}/documents`}>{t("heading")}</Link>
          </nav>
          <h1>{doc.title}</h1>
        </div>
        <a href={`/api/v1/documents/${doc.key}/file`} className={cx(button.secondary, "max-w-72")}>
          <Icon name="download" />
          <span className="truncate">{t("download", { name: doc.filename })}</span>
        </a>
        {admin && <DraftButton docKey={doc.key} />}
      </PageBar>
      <main className="grid items-start gap-5 px-4 py-4 md:px-5 lg:grid-cols-[minmax(0,1fr)_280px]">
        <div className="flex min-w-0 flex-col gap-4">
          {doc.superseded_by && (
            <p className="flex items-center gap-2 rounded-xl border border-warn-line bg-warn-soft px-3.5 py-2.5 text-[13px] font-semibold text-warn">
              <Icon name="warning" />
              <Link href={`/documents/${doc.superseded_by}`} className="text-warn underline hover:text-warn">{t("supersededBy", { key: doc.superseded_by })}</Link>
            </p>
          )}
          <article className={cx(panel, "flex flex-col gap-7 px-5 py-6 md:px-8 md:py-7")}>
            <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted">
              <span className="font-bold">{doc.key}</span>
              {showsClients(clients.data?.items ?? []) && <ClientChip client={doc.client} coreLabel={t("allClients")} />}
              {t("uploadedBy", { name: doc.uploaded_by, date: dateTime(doc.created_at, locale, timeZone) })}
            </p>
            {doc.sections.map((s) => (
              <section
                key={s.number}
                id={`s-${s.number}`}
                aria-labelledby={`h-${s.number}`}
                className="group max-w-3xl scroll-mt-4 rounded-lg target:bg-accent-soft/50 target:ring-8 target:ring-accent-soft/50"
              >
                <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
                  <h2 id={`h-${s.number}`} className={cx("font-extrabold tracking-[-0.01em]", s.level <= 1 ? "text-xl" : s.level === 2 ? "text-[17px]" : "text-[15px]")}>
                    {numbered(s.number) && <span className="text-muted">{s.number}</span>}
                    {numbered(s.number) && " "}
                    {s.title}
                  </h2>
                  {/* The citation, as Ask writes it; shown on hover where there is a mouse. */}
                  <a
                    href={`#s-${s.number}`}
                    className="text-[11.5px] font-semibold text-muted no-underline hover:text-accent focus-visible:opacity-100 group-hover:opacity-100 [@media(hover:hover)]:opacity-0"
                  >
                    {doc.key}/{s.number}
                  </a>
                </div>
                {s.nodes.length > 0 && (
                  <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {s.nodes.map((n) => (
                      <Link key={n.id} href={`/p/${doc.project_key}/modules/${n.id}`} className={cx(chip, "bg-accent-soft text-accent-strong no-underline hover:text-accent-strong")}>
                        <Icon name="screen" className="size-3.5" />
                        {n.name}
                      </Link>
                    ))}
                  </div>
                )}
                {s.body && <Markdown text={asTables(s.body)} className="mt-2 text-[14.5px] leading-relaxed text-ink-soft" />}
              </section>
            ))}
          </article>
        </div>
        <aside className="flex flex-col gap-4 lg:sticky lg:top-4">
          {admin && (
            <Replace
              docKey={doc.key}
              supersededBy={doc.superseded_by}
              candidates={(docs.data?.items ?? []).filter((d) => d.key !== doc.key && !d.superseded_by)}
            />
          )}
          {admin && doc.drafts.length > 0 && (
            <section aria-labelledby="drafts-title" className={cx(panel, "flex flex-col gap-0.5 p-2")}>
              <h2 id="drafts-title" className={cx(sectionTitle, "px-2.5 pb-1 pt-1.5")}>{t("drafts")}</h2>
              {doc.drafts.map((d) => (
                <Link key={d.id} href={`/tree-drafts/${d.id}`} className="flex items-center gap-2 rounded-lg px-2.5 py-2 text-[13px] font-semibold text-ink no-underline hover:bg-paper hover:text-ink">
                  <Icon name="tree" className="size-4 text-muted" />
                  <span title={dateTime(d.created_at, locale, timeZone)}>{dayOf(d.created_at, locale, timeZone)}</span>
                  <span className="ml-auto">
                    <DraftStatusChip status={d.status} label={t(`status.${d.status}`)} />
                  </span>
                </Link>
              ))}
            </section>
          )}
          <nav aria-labelledby="outline-title" className={cx(panel, "flex max-h-[calc(100vh-2rem)] flex-col gap-0.5 overflow-y-auto p-2")}>
            <h2 id="outline-title" className={cx(sectionTitle, "px-2.5 pb-1 pt-1.5")}>{t("outline")}</h2>
            {doc.sections.map((s) => (
              <a
                key={s.number}
                href={`#s-${s.number}`}
                className={cx("flex gap-2 rounded-lg py-1.5 pr-2 text-[13px] no-underline hover:bg-well hover:text-ink", s.level <= 2 ? "font-semibold text-ink" : "text-ink-soft")}
                style={{ paddingLeft: `${0.625 + Math.max(0, s.level - 2) * 0.75}rem` }}
              >
                {numbered(s.number) && <span className="shrink-0 tabular-nums text-muted">{s.number}</span>}
                <span className="min-w-0 truncate">{s.title}</span>
              </a>
            ))}
          </nav>
        </aside>
      </main>
    </>
  );
}
