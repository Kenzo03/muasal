"use client";

import { useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { typeIcon } from "@/components/Chips";
import Icon from "@/components/Icon";
import NodePicker from "@/components/NodePicker";
import { api } from "@/lib/api";
import {
  problemKey, useProblemText,
  type Client, type Contact, type Node, type Priority, type Ref, type Ticket, type TicketType,
} from "@/lib/problem";
import { button, choice, cx, field } from "@/lib/ui";
import { pasteImages } from "@/lib/paste";
import { isWeak } from "@/lib/weak";

type Props = {
  projectKey: string;
  clients: Client[];
  nodes: Node[];
  assignees: Ref[];
  ticket?: Ticket; // edit this ticket; without it the form creates one
  statusId?: number; // the board column a new ticket starts in
  nodeId?: number; // the menu a new ticket starts with (the node page's "New ticket for this menu")
  from?: Prefill; // a new ticket's fields from elsewhere, such as a note's action item (MSL-11)
  onSaved?: () => void;
  onCancel?: () => void;
};

// MSL-11: what a note's action item fills in; noteKey links the ticket to that note.
export type Prefill = { title?: string; assigneeId?: number; due?: string; clientId?: number | null; nodeIds?: number[]; reason?: string; noteKey?: string };

const types: TicketType[] = ["change_request", "bug", "feature"];
const priorities: Priority[] = ["low", "medium", "high", "urgent"];
const lastClientKey = (projectKey: string) => `muasal:last-client:${projectKey}`;

// One labeled field of the form: the label above, the field and its hints below.
function Row({ label, htmlFor, id, children }: { label: string; htmlFor?: string; id?: string; children: React.ReactNode }) {
  const text = "text-[13px] font-bold";
  return (
    <div className="flex min-w-0 flex-col gap-2">
      {htmlFor ? <label htmlFor={htmlFor} className={text}>{label}</label> : <span id={id} className={text}>{label}</span>}
      {children}
    </div>
  );
}

// The one form a PM fills while the client is on the phone (FSD §8.3):
// Client → Requested by → Title → Affected menus → Reason → Type → Description, and More.
export default function TicketForm({ projectKey, clients, nodes, assignees, ticket, statusId, nodeId, from, onSaved, onCancel }: Props) {
  const t = useTranslations("ticketForm");
  const tTypes = useTranslations("ticketTypes");
  const tPri = useTranslations("priorities");
  const problemText = useProblemText();
  const router = useRouter();
  // Editing keeps a user requester; a contact requester can switch to the reporter.
  const userRequester = ticket
    ? ticket.requester.kind === "user" ? { id: ticket.requester.id, name: ticket.requester.name } : ticket.reporter
    : undefined;
  const [clientId, setClientId] = useState<number | null>(ticket?.client?.id ?? from?.clientId ?? null);
  const [requester, setRequester] = useState<"user" | "contact">(ticket?.requester.kind === "contact" ? "contact" : "user");
  const [contactId, setContactId] = useState<number | null>(ticket?.requester.kind === "contact" ? ticket.requester.id : null);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [adding, setAdding] = useState(false);
  const [newName, setNewName] = useState("");
  const [newTitle, setNewTitle] = useState("");
  const [nodeIds, setNodeIds] = useState<Set<number>>(() => new Set(ticket ? ticket.nodes.map((n) => n.id) : from?.nodeIds?.length ? from.nodeIds : nodeId ? [nodeId] : []));
  const [recent, setRecent] = useState<number[]>([]);
  useEffect(() => {
    // Recently used menus first (§8.1); the picker works without them.
    api.GET("/projects/{key}/nodes/recent", { params: { path: { key: projectKey } } }).then(({ data }) => setRecent(data?.node_ids ?? []));
  }, [projectKey]);
  const [reason, setReason] = useState(ticket?.reason ?? from?.reason ?? "");
  const [type, setType] = useState<TicketType>(ticket?.type ?? "change_request");
  const [error, setError] = useState("");
  const [stale, setStale] = useState(false);
  const [notice, setNotice] = useState<ReactNode>("");

  // A new ticket starts with the client picked last time (FSD §8.3).
  useEffect(() => {
    if (ticket || from) return; // a prefill brings its own client
    try {
      const last = localStorage.getItem(lastClientKey(projectKey));
      if (last && last !== "core" && clients.some((c) => c.id === Number(last))) setClientId(Number(last));
    } catch {
      // storage is a convenience
    }
  }, [ticket, from, projectKey, clients]);

  // The contacts of the chosen client, plus internal people.
  useEffect(() => {
    let live = true;
    (async () => {
      const own = clientId === null ? [] : ((await api.GET("/contacts", { params: { query: { client_id: clientId } } })).data?.items ?? []);
      const internal = (await api.GET("/contacts", { params: { query: { internal: true } } })).data?.items ?? [];
      if (live) setContacts([...own, ...internal]);
    })();
    return () => {
      live = false;
    };
  }, [clientId]);

  // MSL-67: the project's releases, when it has any.
  const [releases, setReleases] = useState<{ id: number; name: string }[]>([]);
  useEffect(() => {
    let live = true;
    api.GET("/projects/{key}/releases", { params: { path: { key: projectKey } } }).then(({ data }) => live && data && setReleases(data.items));
    return () => {
      live = false;
    };
  }, [projectKey]);

  // MSL-22: only people who may see the chosen client's tickets can own this one.
  const [people, setPeople] = useState(assignees);
  useEffect(() => {
    if (clientId === null) return setPeople(assignees);
    let live = true;
    api
      .GET("/projects/{key}/assignees", { params: { path: { key: projectKey }, query: { client_id: clientId } } })
      .then(({ data }) => live && data && setPeople(data.items));
    return () => {
      live = false;
    };
  }, [clientId, projectKey, assignees]);

  // R-MR-10: warn, without blocking, when a chosen menu belongs to other clients only.
  const warnings = clientId === null ? [] : nodes.filter((n) => nodeIds.has(n.id) && n.client_specific && !n.clients.some((c) => c.id === clientId));

  const toggleNode = (id: number, on: boolean) =>
    setNodeIds((s) => {
      const next = new Set(s);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  async function addContact() {
    const { data, error } = await api.POST("/contacts", {
      body: { name: newName, title: newTitle || undefined, client_id: clientId ?? undefined },
    });
    if (error) return setError(problemText(error));
    setContacts((cs) => [...cs, data]);
    setContactId(data.id);
    setAdding(false);
    setNewName("");
    setNewTitle("");
    setError("");
  }

  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const another = (e.nativeEvent as SubmitEvent).submitter?.getAttribute("value") === "another";
    const formEl = e.currentTarget;
    const form = new FormData(formEl);
    if (requester === "contact" && contactId === null) return setError(t("chooseContact"));
    const due = String(form.get("due_date") ?? "");
    const estimate = String(form.get("estimate_hours") ?? "");
    const labels = String(form.get("labels") ?? "").split(",").map((l) => l.trim()).filter(Boolean); // MSL-56
    const assignee = String(form.get("assignee_id") ?? "");
    const body = {
      type: String(form.get("type")) as TicketType,
      title: String(form.get("title")),
      client_id: clientId ?? undefined,
      requester_contact_id: requester === "contact" ? (contactId ?? undefined) : undefined,
      requester_user_id: requester === "user" ? userRequester?.id : undefined,
      node_ids: [...nodeIds],
      reason: String(form.get("reason") ?? ""),
      description: String(form.get("description") ?? ""),
      assignee_id: assignee ? Number(assignee) : undefined,
      priority: String(form.get("priority")) as Priority,
      due_date: due || undefined,
      estimate_hours: estimate ? Number(estimate) : undefined, // MSL-54
      labels,
      // MSL-67: before the releases load there is no menu, so an edit keeps the ticket's.
      release_id: form.has("release_id") ? Number(form.get("release_id")) || undefined : ticket?.release?.id,
    };
    if (ticket) {
      const { error } = await api.PUT("/tickets/{key}", {
        params: { path: { key: ticket.key }, header: { "If-Match": `"${ticket.version}"` } },
        body,
      });
      if (error) {
        setStale(problemKey(error) === "stale"); // AC-TK-5: the user's text stays in the form
        return setError(problemText(error));
      }
      onSaved?.();
      return;
    }
    const { data, error } = await api.POST("/projects/{key}/tickets", {
      params: { path: { key: projectKey } },
      body: { ...body, status_id: statusId, note_key: from?.noteKey },
    });
    if (error) return setError(problemText(error));
    try {
      localStorage.setItem(lastClientKey(projectKey), clientId === null ? "core" : String(clientId));
    } catch {
      // storage is a convenience
    }
    if (another) {
      formEl.reset();
      setNodeIds(new Set());
      setReason("");
      setError("");
      setNotice(t.rich("created", { key: data.key, link: (key) => <Link href={`/t/${data.key}`}>{key}</Link> })); // MSL-32
      return;
    }
    router.push(`/t/${data.key}`);
  }

  return (
    <form aria-label={ticket ? t("editTitle", { key: ticket.key }) : t("newTitle")} onSubmit={submit} className="flex flex-col">
      <div className="flex flex-col gap-5 p-5 md:p-6">
        <div className="grid gap-5 md:grid-cols-2">
          <Row label={t("client")} htmlFor="tf-client">
            <select
              id="tf-client"
              value={clientId ?? ""}
              onChange={(e) => {
                setClientId(e.target.value ? Number(e.target.value) : null);
                setContactId(null);
              }}
              className={field.input}
            >
              <option value="">{t("core")}</option>
              {clients.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          </Row>
          <Row label={t("requestedBy")} id="tf-requester">
            <div role="radiogroup" aria-labelledby="tf-requester" className="flex flex-wrap gap-2">
              {/* Explicit label ids: some assistive tech misses wrapping labels (MSL-62). */}
              <label htmlFor="tf-requester-user" className={choice}>
                <input id="tf-requester-user" type="radio" name="requester" value="user" checked={requester === "user"} onChange={() => setRequester("user")} />
                {userRequester?.name ?? t("me")}
              </label>
              <label htmlFor="tf-requester-contact" className={choice}>
                <input id="tf-requester-contact" type="radio" name="requester" value="contact" checked={requester === "contact"} onChange={() => setRequester("contact")} />
                {t("contact")}
              </label>
            </div>
          </Row>
        </div>
        {requester === "contact" && (
          <div className="flex flex-col gap-3 rounded-xl bg-well p-3.5">
            <div className="flex flex-wrap items-center gap-3">
              <select
                aria-label={t("contactSelect")}
                value={contactId ?? ""}
                onChange={(e) => setContactId(e.target.value ? Number(e.target.value) : null)}
                className={cx(field.input, "min-w-0 flex-1")}
              >
                <option value="">{t("chooseContact")}</option>
                {contacts.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                    {c.title ? ` (${c.title})` : ""}
                    {c.client_name ? ` · ${c.client_name}` : ""}
                  </option>
                ))}
              </select>
              {!adding && (
                <button type="button" onClick={() => setAdding(true)} className={button.quiet}>{t("addContact")}</button>
              )}
            </div>
            {adding && (
              <div className="flex flex-wrap items-end gap-3">
                <label className={cx(field.label, "min-w-40 flex-1")}>
                  {t("contactName")}
                  <input value={newName} onChange={(e) => setNewName(e.target.value)} maxLength={200} className={field.compact} />
                </label>
                <label className={cx(field.label, "min-w-40 flex-1")}>
                  {t("contactTitle")}
                  <input value={newTitle} onChange={(e) => setNewTitle(e.target.value)} maxLength={200} className={field.compact} />
                </label>
                <button type="button" onClick={addContact} disabled={!newName.trim()} className={button.secondary}>
                  {t("saveContact")}
                </button>
              </div>
            )}
          </div>
        )}
        <Row label={t("title")} htmlFor="tf-title">
          <input id="tf-title" name="title" defaultValue={ticket?.title ?? from?.title} required minLength={5} maxLength={200} className={field.input} />
        </Row>
        <Row label={t("menus")} id="tf-menus">
          <NodePicker nodes={nodes} selected={nodeIds} onToggle={toggleNode} legend={t("menus")} recent={recent} />
          <p className={field.hint}>{t("menusHint")}</p>
          {warnings.map((n) => (
            <p key={n.id} className="flex items-center gap-1.5 text-xs text-warn">
              <Icon name="warning" className="size-3.5" />
              {t("menuForOtherClients", { menu: n.name, clients: n.clients.map((c) => c.name).join(", ") })}
            </p>
          ))}
        </Row>
        <Row label={t("reason")} htmlFor="tf-reason">
          <textarea
            id="tf-reason"
            name="reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            maxLength={2000}
            rows={3}
            aria-describedby="reason-hint"
            className={field.textarea}
          />
          <p id="reason-hint" className={isWeak(reason) ? "text-xs text-warn" : field.hint}>
            {isWeak(reason) ? t("weakReason") : t("reasonHint")}
          </p>
        </Row>
        <Row label={t("type")} id="tf-type">
          <div role="radiogroup" aria-labelledby="tf-type" className="flex flex-wrap gap-2" onChange={(e) => setType((e.target as HTMLInputElement).value as TicketType)}>
            {types.map((ty) => (
              <label key={ty} htmlFor={`tf-type-${ty}`} className={choice}>
                <input id={`tf-type-${ty}`} type="radio" name="type" value={ty} defaultChecked={(ticket?.type ?? "change_request") === ty} />
                <Icon name={typeIcon[ty][0]} className={cx("size-3.5", typeIcon[ty][1])} />
                {tTypes(ty)}
              </label>
            ))}
          </div>
          {type === "bug" && nodeIds.size > 0 && (
            // A "bug" may be the agreed behavior for this client: the menu's Behaviors tab says (§7.4, story 5).
            <p className={field.hint}>
              {t("bugBehaviors")}{" "}
              {nodes
                .filter((n) => nodeIds.has(n.id))
                .map((n, i) => (
                  <span key={n.id}>
                    {i > 0 && ", "}
                    <a href={`/p/${projectKey}/modules/${n.id}?tab=behaviors`} target="_blank" rel="noopener">{n.name}</a>
                  </span>
                ))}
            </p>
          )}
        </Row>
        <Row label={t("description")} htmlFor="tf-description">
          <textarea
            id="tf-description"
            name="description"
            defaultValue={ticket?.description}
            maxLength={50000}
            rows={5}
            aria-describedby="description-hint"
            onPaste={
              ticket
                ? pasteImages(ticket.key, (p) => setError(problemText(p)), () => router.refresh())
                : (e) => {
                    // A new ticket has nowhere to keep a file yet.
                    if (Array.from(e.clipboardData.files).some((f) => f.type.startsWith("image/"))) {
                      e.preventDefault();
                      setNotice(t("pasteAfterCreate"));
                    }
                  }
            }
            className={field.textarea}
          />
          <p id="description-hint" className={field.hint}>{t(ticket ? "descriptionHint" : "descriptionHintNew")}</p>
        </Row>
        {/* MSL-58: set on nearly every ticket, so in view rather than under "More". */}
        <div className="grid gap-3 md:grid-cols-4">
          <label htmlFor="tf-assignee" className={field.label}>
            {t("assignee")}
            <select id="tf-assignee" name="assignee_id" defaultValue={ticket?.assignee?.id ?? from?.assigneeId ?? ""} className={field.input}>
              <option value="">{t("nobody")}</option>
              {people.map((a) => (
                <option key={a.id} value={a.id}>{a.name}</option>
              ))}
            </select>
          </label>
          <label htmlFor="tf-priority" className={field.label}>
            {t("priority")}
            <select id="tf-priority" name="priority" defaultValue={ticket?.priority ?? "medium"} className={field.input}>
              {priorities.map((p) => (
                <option key={p} value={p}>{tPri(p)}</option>
              ))}
            </select>
          </label>
          <label htmlFor="tf-due" className={field.label}>
            {t("due")}
            <input id="tf-due" type="date" name="due_date" defaultValue={ticket?.due_date ?? from?.due ?? ""} className={field.input} />
          </label>
          <label htmlFor="tf-estimate" className={field.label}>
            {t("estimate")}
            <input id="tf-estimate" type="number" name="estimate_hours" min={0} max={9999} step={0.5} defaultValue={ticket?.estimate_hours ?? ""} className={field.input} />
          </label>
          {releases.length > 0 && (
            <label htmlFor="tf-release" className={field.label}>
              {t("release")}
              <select id="tf-release" name="release_id" defaultValue={ticket?.release?.id ?? ""} className={field.input}>
                <option value="">{t("noRelease")}</option>
                {releases.map((r) => (
                  <option key={r.id} value={r.id}>{r.name}</option>
                ))}
              </select>
            </label>
          )}
          <label htmlFor="tf-labels" className={cx(field.label, releases.length > 0 ? "md:col-span-3" : "md:col-span-4")}>
            {t("labels")}
            <input id="tf-labels" name="labels" defaultValue={ticket?.labels?.join(", ") ?? ""} placeholder={t("labelsHint")} className={field.input} />
          </label>
        </div>
        {error && (
          <p role="alert" className={field.error}>
            {error}{" "}
            {stale && (
              <button type="button" onClick={() => router.refresh()} className={button.quiet}>{t("reload")}</button>
            )}
          </p>
        )}
        {notice && <p role="status" className="text-sm text-ok">{notice}</p>}
      </div>
      {/* The buttons stay in view while a long form scrolls. */}
      <div className="sticky bottom-0 z-10 flex flex-wrap items-center gap-2 rounded-b-2xl border-t border-line-soft bg-paper px-5 py-3.5 md:px-6">
        {ticket ? (
          <>
            <button className={button.primary}>{t("save")}</button>
            <button type="button" onClick={onCancel} className={button.secondary}>{t("cancel")}</button>
          </>
        ) : (
          <>
            <button value="create" className={button.primary}>{t("create")}</button>
            <button value="another" className={button.secondary}>{t("createAnother")}</button>
          </>
        )}
      </div>
    </form>
  );
}
