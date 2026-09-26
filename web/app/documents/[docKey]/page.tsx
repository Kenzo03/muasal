import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip } from "@/components/Chips";
import Markdown from "@/components/Markdown";
import PageBar from "@/components/PageBar";
import { utc } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { button, chip, cx, panel, sectionTitle } from "@/lib/ui";
import DraftButton from "./DraftButton";

// A document page (R-MR-15): the Markdown by section, each with an anchor so
// citation chips such as HRIS-DOC1/7.4 open at the right place, the nodes each
// section produced, the original file and the tree drafts.
export default async function DocumentPage({ params }: { params: Promise<{ docKey: string }> }) {
  const { docKey } = await params;
  const api = await serverApi();
  const { data: doc } = await api.GET("/documents/{key}", { params: { path: { key: docKey } } });
  if (!doc) notFound();
  const project = await getProject(doc.project_key);
  const t = await getTranslations("documents");
  const locale = await getLocale();
  const admin = project?.role === "admin";
  return (
    <>
      <PageBar>
        <span className="font-mono text-[13px] font-semibold text-muted">{doc.key}</span>
        <h1 className="text-base font-semibold">{doc.title}</h1>
        <a href={`/api/v1/documents/${doc.key}/file`} className={cx(button.secondary, "ml-auto")}>{t("download", { name: doc.filename })}</a>
        {admin && <DraftButton docKey={doc.key} />}
      </PageBar>
      <main className="mx-auto grid max-w-6xl gap-4 px-4 py-4 md:px-5 lg:grid-cols-[1fr_260px]">
        <article className={cx(panel, "flex flex-col gap-4 p-5")}>
          <p className="flex flex-wrap items-center gap-2 text-xs text-muted">
            <ClientChip client={doc.client} coreLabel={t("allClients")} />
            {t("uploadedBy", { name: doc.uploaded_by, date: utc(doc.created_at, locale) })}
            {doc.superseded_by && (
              <Link href={`/documents/${doc.superseded_by}`} className={cx(chip, "bg-warn-soft text-warn")}>{t("supersededBy", { key: doc.superseded_by })}</Link>
            )}
          </p>
          {doc.sections.map((s) => (
            <section key={s.number} id={`s-${s.number}`} aria-labelledby={`h-${s.number}`} className="scroll-mt-4 target:rounded target:bg-accent-soft/40">
              <div className="flex flex-wrap items-baseline gap-2">
                <h2 id={`h-${s.number}`} className={cx("font-semibold", s.level <= 2 ? "text-base" : "text-sm")}>
                  {/^s\d/.test(s.number) ? "" : `${s.number} `}{s.title}
                </h2>
                <a href={`#s-${s.number}`} className="font-mono text-[11px] text-muted">{doc.key}/{s.number}</a>
                {s.nodes.map((n) => (
                  <Link key={n.id} href={`/p/${doc.project_key}/modules/${n.id}`} className={cx(chip, "bg-accent-soft text-accent-strong")}>{n.name}</Link>
                ))}
              </div>
              {s.body && <Markdown text={s.body} className="mt-1" />}
            </section>
          ))}
        </article>
        <aside className="flex flex-col gap-3">
          <section aria-labelledby="outline-title" className={cx(panel, "flex flex-col gap-1 p-4")}>
            <h2 id="outline-title" className={sectionTitle}>{t("outline")}</h2>
            {doc.sections.map((s) => (
              <a key={s.number} href={`#s-${s.number}`} className="truncate text-[13px]" style={{ paddingLeft: `${Math.max(0, s.level - 1) * 10}px` }}>
                {s.title}
              </a>
            ))}
          </section>
          {admin && doc.drafts.length > 0 && (
            <section aria-labelledby="drafts-title" className={cx(panel, "flex flex-col gap-1 p-4")}>
              <h2 id="drafts-title" className={sectionTitle}>{t("drafts")}</h2>
              {doc.drafts.map((d) => (
                <Link key={d.id} href={`/tree-drafts/${d.id}`} className="text-[13px]">
                  {utc(d.created_at, locale)} · {t(`status.${d.status}`)}
                </Link>
              ))}
            </section>
          )}
        </aside>
      </main>
    </>
  );
}
