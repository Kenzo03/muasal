import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import { ClientChip } from "@/components/Chips";
import Icon from "@/components/Icon";
import Markdown from "@/components/Markdown";
import NoteForm from "@/components/NoteForm";
import PageBar from "@/components/PageBar";
import { actionItems } from "@/lib/actions";
import { dateTime, day } from "@/lib/format";
import { getProject, serverApi } from "@/lib/server-api";
import { button, cx, panel, sectionTitle } from "@/lib/ui";
import CreateAll from "./CreateAll";

// One decision note (FSD §9.4). The author and project admins edit it; every
// edit is in the audit log.
export default async function NotePage({ params, searchParams }: {
  params: Promise<{ noteKey: string }>;
  searchParams: Promise<{ edit?: string }>;
}) {
  const { noteKey } = await params;
  const { edit } = await searchParams;
  const api = await serverApi();
  const { data: note } = await api.GET("/notes/{noteKey}", { params: { path: { noteKey } } });
  if (!note) notFound();
  const t = await getTranslations("notes");
  const locale = await getLocale();
  const timeZone = await getTimeZone();
  const editing = edit === "1" && note.can_edit;
  // MSL-11: each action item can become an assigned ticket, linked back here.
  const actions = editing ? [] : actionItems(note.body, Number(note.decided_on.slice(0, 4)));
  const people = actions.length === 0 ? [] : ((await api.GET("/projects/{key}/assignees", {
    params: { path: { key: note.project_key }, query: note.client ? { client_id: note.client.id } : {} },
  })).data?.items ?? []);
  const ownerId = (owner?: string) => {
    const o = owner?.toLowerCase().trim();
    const hits = o ? people.filter((p) => p.name.toLowerCase() === o || p.name.toLowerCase().split(/\s+/)[0] === o) : [];
    return hits.length === 1 ? hits[0].id : undefined;
  };
  const filed = (task: string) => note.tickets.find((tk) => tk.title.toLowerCase() === task.toLowerCase());
  // One action item as a new ticket: the form's defaults plus what the item says.
  const draft = (a: (typeof actions)[number]) => ({
    type: "change_request" as const, priority: "medium" as const, title: a.task.slice(0, 200),
    reason: t("actionReason", { key: note.key, title: note.title }), note_key: note.key,
    assignee_id: ownerId(a.owner), due_date: a.due, client_id: note.client?.id, node_ids: note.nodes.map((n) => n.id),
  });
  const newTicket = (a: (typeof actions)[number]) => {
    const d = draft(a);
    const q: Record<string, string> = { title: d.title, reason: d.reason, note: note.key };
    if (d.assignee_id) q.assignee = String(d.assignee_id);
    if (d.due_date) q.due = d.due_date;
    if (d.client_id) q.client = String(d.client_id);
    if (d.node_ids.length > 0) q.nodes = d.node_ids.join(",");
    return `/p/${note.project_key}/tickets/new?${new URLSearchParams(q)}`;
  };
  const unfiled = actions.filter((a) => !filed(a.task)).map(draft);
  const canFile = unfiled.length > 1 && (await getProject(note.project_key))?.role !== "viewer";
  let form: React.ReactNode = null;
  if (editing) {
    const path = { params: { path: { key: note.project_key } } };
    const [clients, nodes] = await Promise.all([api.GET("/projects/{key}/clients", path), api.GET("/projects/{key}/nodes", path)]);
    form = <NoteForm projectKey={note.project_key} note={note} clients={clients.data?.items ?? []} nodes={nodes.data?.items ?? []} />;
  }
  return (
    <>
      <PageBar>
        <nav aria-label={t("path")} className="flex items-center gap-1.5 text-[13px] text-muted">
          <Link href={`/p/${note.project_key}/board`}>{note.project_key}</Link>
          <Icon name="chevronRight" className="size-3.5" />
          <Link href={`/p/${note.project_key}/notes`}>{t("heading")}</Link>
          <Icon name="chevronRight" className="size-3.5" />
          <span className="font-mono font-semibold text-ink">{note.key}</span>
        </nav>
        {note.can_edit && !editing && (
          <Link href="?edit=1" className={cx(button.secondary, "ml-auto")}>
            <Icon name="edit" />
            {t("edit")}
          </Link>
        )}
      </PageBar>
      <main className="flex flex-col gap-4 px-4 py-4 md:px-5">
        {editing ? (
          <div className={panel}>{form}</div>
        ) : (
          <>
            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted">
                <span className="font-mono font-semibold">{note.key}</span>
                <span>{t("decidedOnDate", { date: day(note.decided_on, locale) })}</span>
                <ClientChip client={note.client} coreLabel={t("allClients")} />
                {note.archived && <span>{t("archivedLabel")}</span>}
              </div>
              <h1 className="text-2xl font-semibold leading-tight">{note.title}</h1>
            </div>
            <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_340px]">
              <section aria-label={t("body")} className={cx(panel, "px-4 py-3.5")}>
                <Markdown text={note.body} />
              </section>
              <aside className={cx(panel, "flex flex-col gap-3 px-4 py-3.5 text-[13px]")}>
                <div>
                  <h2 className={sectionTitle}>{t("attendees")}</h2>
                  <p>{note.attendees || "—"}</p>
                </div>
                <div>
                  <h2 className={sectionTitle}>{t("menus")}</h2>
                  {note.nodes.length === 0 && <p>{t("wholeProject")}</p>}
                  <ul className="flex flex-wrap gap-1.5">
                    {note.nodes.map((n) => (
                      <li key={n.id}><Link href={`/p/${note.project_key}/modules/${n.id}`}>{n.name}</Link></li>
                    ))}
                  </ul>
                </div>
                {actions.length > 0 && (
                  <div>
                    <h2 className={sectionTitle}>{t("actionItems")}</h2>
                    <ul className="flex flex-col gap-2">
                      {actions.map((a, i) => {
                        const done = filed(a.task);
                        return (
                          <li key={i} className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                            <span className="min-w-0 flex-[1_1_12rem]">
                              {a.owner && <span className="font-semibold">{a.owner}: </span>}
                              {a.task}
                              {a.due && <span className="text-muted"> · {day(a.due, locale)}</span>}
                            </span>
                            {done ? (
                              <Link href={`/t/${done.key}`} className="font-mono text-xs font-semibold">{done.key}</Link>
                            ) : (
                              <Link href={newTicket(a)} className="text-xs font-semibold">{t("createTicket")}</Link>
                            )}
                          </li>
                        );
                      })}
                    </ul>
                    {canFile && <CreateAll projectKey={note.project_key} tickets={unfiled} />}
                  </div>
                )}
                <div>
                  <h2 className={sectionTitle}>{t("tickets")}</h2>
                  {note.tickets.length === 0 ? <p className="text-muted">—</p> : (
                    <ul className="flex flex-col gap-1">
                      {note.tickets.map((tk) => (
                        <li key={tk.key}><Link href={`/t/${tk.key}`} className="font-mono font-semibold">{tk.key}</Link> {tk.title}</li>
                      ))}
                    </ul>
                  )}
                </div>
                <p className="text-xs text-muted">{t("byline", { name: note.author.name, at: dateTime(note.updated_at, locale, timeZone) })}</p>
              </aside>
            </div>
          </>
        )}
      </main>
    </>
  );
}
