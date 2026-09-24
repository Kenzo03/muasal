"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import {
  problemKey, useProblemText,
  type Client, type Contact, type Node, type Priority, type Ref, type Ticket, type TicketType,
} from "@/lib/problem";

type Props = {
  projectKey: string;
  clients: Client[];
  nodes: Node[];
  assignees: Ref[];
  ticket?: Ticket; // edit this ticket; without it the form creates one
  statusId?: number; // the board column a new ticket starts in
  onSaved?: () => void;
  onCancel?: () => void;
};

const types: TicketType[] = ["change_request", "bug", "feature"];
const priorities: Priority[] = ["low", "medium", "high", "urgent"];
const lastClientKey = (projectKey: string) => `muasal:last-client:${projectKey}`;

// The one form a PM fills while the client is on the phone (FSD §8.3):
// Client → Requested by → Title → Affected menus → Reason → Type → Description, and More.
export default function TicketForm({ projectKey, clients, nodes, assignees, ticket, statusId, onSaved, onCancel }: Props) {
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
  const [nodeIds, setNodeIds] = useState<Set<number>>(() => new Set(ticket?.nodes.map((n) => n.id)));
  const [menuFilter, setMenuFilter] = useState("");
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

  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const pathOf = (n: Node) => {
    const names = [n.name];
    for (let p = n.parent_id === null ? undefined : byId.get(n.parent_id); p; p = p.parent_id === null ? undefined : byId.get(p.parent_id)) {
      names.unshift(p.name);
    }
    return names.join(" › ");
  };
  const q = menuFilter.trim().toLowerCase();
  const menuChoices = nodes.filter((n) => !q || [pathOf(n), n.code ?? "", ...n.aliases].some((s) => s.toLowerCase().includes(q)));
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
      setError("");
      setNotice(t("created", { key: data.key }));
      return;
    }
    router.push(`/t/${data.key}`);
  }

  const input = "rounded border px-3 py-2";
  return (
    <form aria-label={ticket ? t("editTitle", { key: ticket.key }) : t("newTitle")} onSubmit={submit} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1 text-sm">
        {t("client")}
        <select
          value={clientId ?? ""}
          onChange={(e) => {
            setClientId(e.target.value ? Number(e.target.value) : null);
            setContactId(null);
          }}
          className={input}
        >
          <option value="">{t("core")}</option>
          {clients.map((c) => (
            <option key={c.id} value={c.id}>{c.name}</option>
          ))}
        </select>
      </label>
      <fieldset className="flex flex-col gap-2 text-sm">
        <legend className="mb-1">{t("requestedBy")}</legend>
        <label className="flex items-center gap-2">
          <input type="radio" name="requester" checked={requester === "user"} onChange={() => setRequester("user")} />
          {userRequester?.name ?? t("me")}
        </label>
        <label className="flex items-center gap-2">
          <input type="radio" name="requester" checked={requester === "contact"} onChange={() => setRequester("contact")} />
          {t("contact")}
        </label>
        {requester === "contact" && (
          <div className="ml-6 flex flex-col gap-2">
            <label className="flex flex-col gap-1">
              {t("contactSelect")}
              <select value={contactId ?? ""} onChange={(e) => setContactId(e.target.value ? Number(e.target.value) : null)} className={input}>
                <option value="">{t("chooseContact")}</option>
                {contacts.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                    {c.title ? ` (${c.title})` : ""}
                    {c.client_name ? ` · ${c.client_name}` : ""}
                  </option>
                ))}
              </select>
            </label>
            {adding ? (
              <div className="flex flex-wrap items-end gap-2">
                <label className="flex flex-col gap-1">
                  {t("contactName")}
                  <input value={newName} onChange={(e) => setNewName(e.target.value)} maxLength={200} className={input} />
                </label>
                <label className="flex flex-col gap-1">
                  {t("contactTitle")}
                  <input value={newTitle} onChange={(e) => setNewTitle(e.target.value)} maxLength={200} className={input} />
                </label>
                <button type="button" onClick={addContact} disabled={!newName.trim()} className="rounded border px-3 py-2">
                  {t("saveContact")}
                </button>
              </div>
            ) : (
              <button type="button" onClick={() => setAdding(true)} className="self-start underline">{t("addContact")}</button>
            )}
          </div>
        )}
      </fieldset>
      <label className="flex flex-col gap-1 text-sm">
        {t("title")}
        <input name="title" defaultValue={ticket?.title} required minLength={5} maxLength={200} className={input} />
      </label>
      <fieldset className="flex flex-col gap-2 text-sm">
        <legend className="mb-1">{t("menus")}</legend>
        <input value={menuFilter} onChange={(e) => setMenuFilter(e.target.value)} aria-label={t("menusFilter")} placeholder={t("menusFilter")} className={input} />
        <div className="flex max-h-48 flex-col gap-1 overflow-y-auto rounded border p-2">
          {menuChoices.map((n) => (
            <label key={n.id} className="flex items-center gap-2">
              <input type="checkbox" checked={nodeIds.has(n.id)} onChange={(e) => toggleNode(n.id, e.target.checked)} />
              {pathOf(n)}
            </label>
          ))}
        </div>
        <p className="text-xs text-neutral-500">{t("menusHint")}</p>
        {warnings.map((n) => (
          <p key={n.id} className="text-xs text-amber-700">
            {t("menuForOtherClients", { menu: n.name, clients: n.clients.map((c) => c.name).join(", ") })}
          </p>
        ))}
      </fieldset>
      <label className="flex flex-col gap-1 text-sm">
        {t("reason")}
        <textarea name="reason" defaultValue={ticket?.reason} maxLength={2000} rows={3} aria-describedby="reason-hint" className={input} />
      </label>
      <p id="reason-hint" className="-mt-3 text-xs text-neutral-500">{t("reasonHint")}</p>
      <label className="flex flex-col gap-1 text-sm">
        {t("type")}
        <select name="type" defaultValue={ticket?.type ?? "change_request"} className={input}>
          {types.map((ty) => (
            <option key={ty} value={ty}>{tTypes(ty)}</option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("description")}
        <textarea name="description" defaultValue={ticket?.description} maxLength={50000} rows={5} className={input} />
      </label>
      <details className="text-sm" open={Boolean(ticket?.assignee || ticket?.due_date)}>
        <summary className="cursor-pointer">{t("more")}</summary>
        <div className="mt-3 flex flex-wrap gap-4">
          <label className="flex flex-col gap-1">
            {t("assignee")}
            <select name="assignee_id" defaultValue={ticket?.assignee?.id ?? ""} className={input}>
              <option value="">{t("nobody")}</option>
              {assignees.map((a) => (
                <option key={a.id} value={a.id}>{a.name}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1">
            {t("priority")}
            <select name="priority" defaultValue={ticket?.priority ?? "medium"} className={input}>
              {priorities.map((p) => (
                <option key={p} value={p}>{tPri(p)}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1">
            {t("due")}
            <input type="date" name="due_date" defaultValue={ticket?.due_date ?? ""} className={input} />
          </label>
        </div>
      </details>
      {error && (
        <p role="alert" className="text-sm text-red-700">
          {error}{" "}
          {stale && (
            <button type="button" onClick={() => router.refresh()} className="underline">{t("reload")}</button>
          )}
        </p>
      )}
      {notice && <p role="status" className="text-sm">{notice}</p>}
      <div className="flex gap-3">
        {ticket ? (
          <>
            <button className="rounded bg-neutral-900 px-4 py-2 text-white">{t("save")}</button>
            <button type="button" onClick={onCancel} className="rounded border px-4 py-2">{t("cancel")}</button>
          </>
        ) : (
          <>
            <button value="create" className="rounded bg-neutral-900 px-4 py-2 text-white">{t("create")}</button>
            <button value="another" className="rounded border px-4 py-2">{t("createAnother")}</button>
          </>
        )}
      </div>
    </form>
  );
}
