"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import NodePicker from "@/components/NodePicker";
import { api } from "@/lib/api";
import {
  problemKey, useProblemText,
  type Client, type Contact, type Node, type Priority, type Ref, type Ticket, type TicketType,
} from "@/lib/problem";
import { button, cx, field } from "@/lib/ui";
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
  onSaved?: () => void;
  onCancel?: () => void;
};

const types: TicketType[] = ["change_request", "bug", "feature"];
const priorities: Priority[] = ["low", "medium", "high", "urgent"];
const lastClientKey = (projectKey: string) => `muasal:last-client:${projectKey}`;

// One labeled row of the form: the label on the left, the field on the right.
function Row({ label, htmlFor, id, children }: { label: string; htmlFor?: string; id?: string; children: React.ReactNode }) {
  const text = "text-sm font-semibold md:pt-1.5";
  return (
    <div className="grid gap-x-5 gap-y-1.5 md:grid-cols-[170px_minmax(0,1fr)] md:items-start">
      {htmlFor ? <label htmlFor={htmlFor} className={text}>{label}</label> : <span id={id} className={text}>{label}</span>}
      <div className="flex min-w-0 flex-col gap-2">{children}</div>
    </div>
  );
}

// The one form a PM fills while the client is on the phone (FSD §8.3):
// Client → Requested by → Title → Affected menus → Reason → Type → Description, and More.
export default function TicketForm({ projectKey, clients, nodes, assignees, ticket, statusId, nodeId, onSaved, onCancel }: Props) {
  const t = useTranslations("ticketForm");
  const tTypes = useTranslations("ticketTypes");
  const tPri = useTranslations("priorities");
  const problemText = useProblemText();
  const router = useRouter();
  // Editing keeps a user requester; a contact requester can switch to the reporter.
  const userRequester = ticket
    ? ticket.requester.kind === "user" ? { id: ticket.requester.id, name: ticket.requester.name } : ticket.reporter
    : undefined;
  const [clientId, setClientId] = useState<number | null>(ticket?.client?.id ?? null);
  const [requester, setRequester] = useState<"user" | "contact">(ticket?.requester.kind === "contact" ? "contact" : "user");
  const [contactId, setContactId] = useState<number | null>(ticket?.requester.kind === "contact" ? ticket.requester.id : null);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [adding, setAdding] = useState(false);
  const [newName, setNewName] = useState("");
  const [newTitle, setNewTitle] = useState("");
const [nodeIds, setNodeIds] = useState<Set<number>>(() => new Set(ticket ? ticket.nodes.map((n) => n.id) : nodeId ? [nodeId] : []));
const [reason, setReason] = useState(ticket?.reason ?? "");
  const [error, setError] = useState("");
  const [stale, setStale] = useState(false);
  const [notice, setNotice] = useState("");

  // A new ticket starts with the client picked last time (FSD §8.3).
  useEffect(() => {
    if (ticket) return;
    try {
      const last = localStorage.getItem(lastClientKey(projectKey));
      if (last && last !== "core" && clients.some((c) => c.id === Number(last))) setClientId(Number(last));
    } catch {
      // storage is a convenience
    }
  }, [ticket, projectKey, clients]);

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
      body: { ...body, status_id: statusId },
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
      setNotice(t("created", { key: data.key }));
      return;
    }
    router.push(`/t/${data.key}`);
  }

  const radio = "size-4 accent-accent";
  return (
    <form aria-label={ticket ? t("editTitle", { key: ticket.key }) : t("newTitle")} onSubmit={submit} className="flex flex-col">
      <div className="flex flex-col gap-4 p-5">
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
          <div role="radiogroup" aria-labelledby="tf-requester" className="flex flex-wrap gap-x-5 gap-y-2 text-sm md:pt-1.5">
            <label className="flex items-center gap-2">
              <input type="radio" name="requester" checked={requester === "user"} onChange={() => setRequester("user")} className={radio} />
              {userRequester?.name ?? t("me")}
            </label>
            <label className="flex items-center gap-2">
              <input type="radio" name="requester" checked={requester === "contact"} onChange={() => setRequester("contact")} className={radio} />
              {t("contact")}
            </label>
          </div>
          {requester === "contact" && (
            <>
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
                <div className="flex flex-wrap items-end gap-3 rounded border border-line-soft bg-paper p-3">
                  <label className={field.label}>
                    {t("contactName")}
                    <input value={newName} onChange={(e) => setNewName(e.target.value)} maxLength={200} className={field.input} />
                  </label>
                  <label className={field.label}>
                    {t("contactTitle")}
                    <input value={newTitle} onChange={(e) => setNewTitle(e.target.value)} maxLength={200} className={field.input} />
                  </label>
                  <button type="button" onClick={addContact} disabled={!newName.trim()} className={button.secondary}>
                    {t("saveContact")}
                  </button>
                </div>
              )}
            </>
          )}
        </Row>
        <Row label={t("title")} htmlFor="tf-title">
          <input id="tf-title" name="title" defaultValue={ticket?.title} required minLength={5} maxLength={200} className={field.input} />
        </Row>
        <Row label={t("menus")} id="tf-menus">
          <NodePicker nodes={nodes} selected={nodeIds} onToggle={toggleNode} legend={t("menus")} />
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
          <div role="radiogroup" aria-labelledby="tf-type" className="flex flex-wrap">
            {types.map((ty, i) => (
              <label
                key={ty}
                className={cx(
                  "flex h-[34px] cursor-pointer items-center gap-2 border border-field px-3 text-[13px] has-[:checked]:z-10 has-[:checked]:border-accent has-[:checked]:bg-accent-soft has-[:checked]:font-semibold has-[:checked]:text-accent-strong",
                  i === 0 ? "rounded-l" : "-ml-px",
                  i === types.length - 1 && "rounded-r",
                )}
              >
                <input type="radio" name="type" value={ty} defaultChecked={(ticket?.type ?? "change_request") === ty} className={radio} />
                {tTypes(ty)}
              </label>
            ))}
          </div>
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
        <details className="group" open={Boolean(ticket?.assignee || ticket?.due_date)}>
          <summary className="flex cursor-pointer items-center gap-1.5 text-sm font-semibold">
            <Icon name="chevronRight" className="size-4 text-muted transition-transform group-open:rotate-90" />
            {t("more")}
          </summary>
          <div className="mt-3 grid gap-3 md:ml-[190px] md:grid-cols-3">
            <label className={field.label}>
              {t("assignee")}
              <select name="assignee_id" defaultValue={ticket?.assignee?.id ?? ""} className={field.input}>
                <option value="">{t("nobody")}</option>
                {assignees.map((a) => (
                  <option key={a.id} value={a.id}>{a.name}</option>
                ))}
              </select>
            </label>
            <label className={field.label}>
              {t("priority")}
              <select name="priority" defaultValue={ticket?.priority ?? "medium"} className={field.input}>
                {priorities.map((p) => (
                  <option key={p} value={p}>{tPri(p)}</option>
                ))}
              </select>
            </label>
            <label className={field.label}>
              {t("due")}
              <input type="date" name="due_date" defaultValue={ticket?.due_date ?? ""} className={field.input} />
            </label>
          </div>
        </details>
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
      <div className="flex flex-wrap items-center gap-2 rounded-b border-t border-line-soft bg-paper px-5 py-3">
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
