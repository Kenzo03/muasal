import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import { showsClients } from "@/components/Chips";
import PageBar from "@/components/PageBar";
import type { components } from "@/lib/api-types";
import { dateIn, day } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { cx, field } from "@/lib/ui";
import StatusTools from "../status/StatusTools";

type HandoverNode = components["schemas"]["HandoverNode"];

// The handover pack (MSL-68): at the end of a project, every module and menu
// with its spec sections, the behaviours in force and what is still open, as
// one printable document that also copies as Markdown.
export default async function HandoverPage({ params, searchParams }: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ client?: string }>;
}) {
  const { key } = await params;
  const { client } = await searchParams;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("handover");
  const locale = await getLocale();
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const clients = (await api.GET("/projects/{key}/clients", path)).data?.items ?? [];
  const picked = clients.find((c) => String(c.id) === client);
  const { data } = await api.GET("/projects/{key}/handover", { params: { path: { key }, query: picked ? { client_id: picked.id } : {} } });
  const nodes = data?.nodes ?? [];
  const title = t("title", { project: project.name, date: day(dateIn(new Date(), await getTimeZone()), locale) }) + (picked ? ` · ${picked.name}` : "");
  const name = (n: HandoverNode) => (n.code ? `${n.name} (${n.code})` : n.name);
  const empty = (n: HandoverNode) => n.sections.length + n.behaviors.length + n.open.length === 0;
  const behavior = (b: HandoverNode["behaviors"][number]) =>
    `${b.what_changed}${b.why ? ` ${t("why")}: ${b.why}` : ""}${b.client ? ` (${b.client.name})` : ""}`;
  const ticket = (o: HandoverNode["open"][number]) =>
    [o.status, o.assignee ?? t("nobody"), o.due_date && t("due", { date: day(o.due_date, locale) })].filter(Boolean).join(", ");

  const markdown = [
    `# ${title}`,
    ...nodes.flatMap((n) => [
      "",
      `${"#".repeat(Math.min(n.depth + 2, 4))} ${name(n)}`,
      ...(n.sections.length ? ["", `**${t("spec")}**`, ...n.sections.map((s) => `- ${s.key} ${s.title}`)] : []),
      ...(n.behaviors.length ? ["", `**${t("behaviors")}**`, ...n.behaviors.map((b) => `- ${b.key}: ${behavior(b)}`)] : []),
      ...(n.open.length ? ["", `**${t("open")}**`, ...n.open.map((o) => `- ${o.key} ${o.title} (${ticket(o)})`)] : []),
      ...(n.type === "menu" && empty(n) ? ["", `_${t("nothing")}_`] : []),
    ]),
  ].join("\n");

  return (
    <>
      <PageBar>
        <h1>{t("heading")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
        <StatusTools markdown={markdown} />
      </PageBar>
      <main className="flex max-w-4xl flex-col gap-4 px-4 py-4 md:px-5">
        <p className="text-[13px] text-muted print:hidden">{t("intro")}</p>
        {showsClients(clients) && (
          <form className="flex items-end gap-2 print:hidden">
            <label htmlFor="handover-client" className={field.label}>
              {t("client")}
              <select id="handover-client" name="client" defaultValue={picked ? String(picked.id) : ""} className={field.compact}>
                <option value="">{t("allClients")}</option>
                {clients.map((c) => (
                  <option key={c.id} value={c.id}>{c.name}</option>
                ))}
              </select>
            </label>
            <button className="h-9 rounded-lg px-3 text-[13px] font-semibold text-accent">{t("show")}</button>
          </form>
        )}
        <h2 className="hidden text-xl font-bold print:block">{title}</h2>
        {nodes.length === 0 && <p className="text-muted">{t("noTree")}</p>}
        {nodes.map((n) => {
          const H = n.depth === 0 ? "h2" : "h3";
          return (
            <section key={n.id} aria-labelledby={`node-${n.id}`} className={cx("flex flex-col gap-2 break-inside-avoid", n.depth > 0 && "border-l-2 border-line-soft pl-4", n.depth === 2 && "ml-4", n.depth > 2 && "ml-8")}>
              <H id={`node-${n.id}`} className={n.depth === 0 ? "text-lg font-extrabold" : "text-[15px] font-bold"}>
                <Link href={`/p/${key}/modules/${n.id}`} className="text-ink no-underline hover:underline">{name(n)}</Link>
              </H>
              {n.sections.length > 0 && (
                <div className="flex flex-col gap-1 text-[13px]">
                  <span className="font-semibold text-muted">{t("spec")}</span>
                  <ul className="flex flex-col gap-0.5">
                    {n.sections.map((s) => (
                      <li key={s.key}><span className="font-mono text-xs font-semibold">{s.key}</span> {s.title}</li>
                    ))}
                  </ul>
                </div>
              )}
              {n.behaviors.length > 0 && (
                <div className="flex flex-col gap-1 text-[13px]">
                  <span className="font-semibold text-muted">{t("behaviors")}</span>
                  <ul className="flex flex-col gap-1">
                    {n.behaviors.map((b) => (
                      <li key={b.key}>
                        <Link href={`/t/${b.key}`} className="font-mono text-xs font-semibold">{b.key}</Link> {behavior(b)}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {n.open.length > 0 && (
                <div className="flex flex-col gap-1 text-[13px]">
                  <span className="font-semibold text-muted">{t("open")}</span>
                  <ul className="flex flex-col gap-0.5">
                    {n.open.map((o) => (
                      <li key={o.key}>
                        <Link href={`/t/${o.key}`} className="font-mono text-xs font-semibold">{o.key}</Link> {o.title} <span className="text-muted">({ticket(o)})</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {n.type === "menu" && empty(n) && <p className="text-[13px] text-muted">{t("nothing")}</p>}
            </section>
          );
        })}
      </main>
    </>
  );
}
