import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { ClientChip, showsClients } from "@/components/Chips";
import Icon from "@/components/Icon";
import PageBar from "@/components/PageBar";
import { day, utc } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { chip, cx, panel } from "@/lib/ui";
import Upload from "./Upload";

// The file's kind as a tile: PDF, DOCX or MD, from its extension.
function FileTile({ filename }: { filename: string }) {
  const ext = filename.split(".").pop()?.toUpperCase() ?? "";
  const tone = ext === "PDF" ? "bg-danger-soft text-danger" : ext === "DOCX" ? "bg-[#DCE8F5] text-[#1F4F82]" : "bg-well text-ink-soft";
  return (
    <span aria-hidden="true" className={cx("flex size-10 shrink-0 items-center justify-center rounded-[10px] text-[10.5px] font-extrabold tracking-wide", tone)}>
      {ext === "MARKDOWN" ? "MD" : ext}
    </span>
  );
}

// Project documents (FSD §7.7): specifications Ask cites by section, and the
// source of AI tree drafts. Project admins upload beside the list.
export default async function DocumentsPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("documents");
  const locale = await getLocale();
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [docs, clients] = await Promise.all([api.GET("/projects/{key}/documents", path), api.GET("/projects/{key}/clients", path)]);
  const withClients = showsClients(clients.data?.items ?? []);
  const items = docs.data?.items ?? [];
  const admin = project.role === "admin";
  return (
    <>
      <PageBar>
        <h1>{t("heading")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
      </PageBar>
      <main className="flex flex-col gap-4 px-4 py-4 md:px-5">
        <p className="max-w-3xl text-[13.5px] text-ink-soft">{t("intro")}</p>
        <div className={cx("grid items-start gap-5", admin && "lg:grid-cols-[minmax(0,1fr)_340px]")}>
          {items.length === 0 ? (
            <div className={cx(panel, "flex flex-col items-center gap-2.5 px-6 py-12 text-center")}>
              <span className="flex size-11 items-center justify-center rounded-full bg-well text-muted">
                <Icon name="file" className="size-5" />
              </span>
              <p className="text-[13.5px] text-ink-soft">{t("none")}</p>
            </div>
          ) : (
            <ul aria-label={t("heading")} className={cx(panel, "flex flex-col gap-0.5 p-1.5")}>
              {items.map((d) => (
                <li key={d.key}>
                  <Link href={`/documents/${d.key}`} className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-ink no-underline hover:bg-paper hover:text-ink">
                    <FileTile filename={d.filename} />
                    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                      <span className={cx("text-sm font-semibold", d.superseded_by && "text-muted")}>{d.title}</span>
                      <span className="text-xs text-muted">
                        <span className="font-bold">{d.key}</span> · <span title={utc(d.created_at, locale)}>{day(d.created_at, locale)}</span> · {d.uploaded_by}
                      </span>
                    </span>
                    {d.superseded_by && <span className={cx(chip, "bg-warn-soft text-warn")}>{t("supersededBy", { key: d.superseded_by })}</span>}
                    {withClients && <ClientChip client={d.client} coreLabel={t("allClients")} />}
                  </Link>
                </li>
              ))}
            </ul>
          )}
          {admin && <Upload projectKey={key} clients={clients.data?.items ?? []} documents={items} />}
        </div>
      </main>
    </>
  );
}
