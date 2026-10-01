"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { ClientChip } from "@/components/Chips";
import ConfirmDialog from "@/components/ConfirmDialog";
import FilterTabs from "@/components/FilterTabs";
import Icon from "@/components/Icon";
import Menu from "@/components/Menu";
import SidePanel from "@/components/SidePanel";
import { matches, maxAliases } from "@/lib/admin";
import { api } from "@/lib/api";
import { useProblemText, type Client, type Problem } from "@/lib/problem";
import { button, chip, cx, field, table } from "@/lib/ui";
import AliasInput from "./AliasInput";

type Filter = "active" | "archived" | "all";
const menuItem = "flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-ink hover:bg-paper";
const inFilter = (f: Filter, c: Client) => f === "all" || (f === "archived") === c.archived;

export default function ClientsAdmin({ clients }: { clients: Client[] }) {
  const t = useTranslations("clients");
  const problemText = useProblemText();
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("active");
  const [panel, setPanel] = useState<Client | "new" | null>(null);
  const [archiving, setArchiving] = useState<Client | null>(null);
  const [error, setError] = useState("");

  const shown = clients.filter((c) => inFilter(filter, c) && matches(query, c.name, c.code, ...c.aliases));

  async function setArchived(c: Client, archived: boolean): Promise<string | undefined> {
    const { error } = await api.PATCH("/clients/{id}", { params: { path: { id: c.id } }, body: { archived } });
    if (error) return problemText(error);
    router.refresh();
  }
  // Restore needs no confirmation. From the menu a failure shows in the page's alert line.
  async function restore(c: Client) {
    setError((await setArchived(c, false)) ?? "");
  }
  // From the panel the failure is returned, to show inside the panel; success closes it.
  async function restoreFromPanel(c: Client) {
    const problem = await setArchived(c, false);
    if (!problem) {
      setError("");
      setPanel(null);
    }
    return problem;
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex h-9 w-full items-center gap-2 rounded-[10px] border border-line bg-white px-3 text-muted sm:w-64">
          <Icon name="search" className="size-[15px]" />
          <input type="search" aria-label={t("search")} placeholder={t("search")} value={query} onChange={(e) => setQuery(e.target.value)} className="min-w-0 flex-1 bg-transparent text-[13.5px] text-ink outline-none" />
        </label>
        <FilterTabs
          label={t("statusFilter")}
          value={filter}
          onChange={setFilter}
          options={(["active", "archived", "all"] as const).map((k) => ({ key: k, label: t(k), count: clients.filter((c) => inFilter(k, c)).length }))}
        />
        <button type="button" className={cx(button.primary, "ml-auto")} onClick={() => setPanel("new")}>
          <Icon name="plus" />
          {t("newClient")}
        </button>
      </div>
      {error && <p role="alert" className={field.error}>{error}</p>}
      {/* md:overflow-visible: the row menu would be clipped by the wrapper's scroll box; phones keep the sideways scroll. */}
      <div className={cx(table.wrap, "md:overflow-visible")}>
        <table className={table.table}>
          <thead className={table.head}>
            <tr>
              <th className={table.th}>{t("name")}</th>
              <th className={table.th}>{t("code")}</th>
              <th className={table.th}>{t("aliases")}</th>
              <th className={table.th}>{t("projects")}</th>
              <th className={table.th}>{t("status")}</th>
              <th className={cx(table.th, "w-14")}><span className="sr-only">{t("actionsFor", { name: "" })}</span></th>
            </tr>
          </thead>
          <tbody>
            {shown.map((c, i) => (
              <tr key={c.id} className={cx(table.row, panel !== "new" && panel?.id === c.id && "bg-accent-soft/40")}>
                <td className={table.td}>
                  <button type="button" onClick={() => setPanel(c)} className="rounded-md hover:ring-2 hover:ring-line">
                    <ClientChip client={c} coreLabel="" />
                  </button>
                </td>
                <td className={cx(table.td, "text-[12.5px] font-extrabold text-muted")}>{c.code ?? ""}</td>
                <td className={table.td}>
                  <span className="flex flex-wrap gap-1">
                    {c.aliases.map((a) => <span key={a} className={cx(chip, "bg-well text-ink-soft")}>{a}</span>)}
                  </span>
                </td>
                <td className={cx(table.td, "text-[12.5px] font-bold text-ink-soft")}>{c.projects?.length ? c.projects.join(" · ") : "—"}</td>
                <td className={table.td}>
                  <span className={cx(chip, c.archived ? "bg-well text-muted" : "bg-ok-soft text-ok")}>{c.archived ? t("archived") : t("active")}</span>
                </td>
                <td className={cx(table.td, "py-1.5 text-right")}>
                  <Menu
                    label={t("actionsFor", { name: c.name })}
                    align="right"
                    side={i >= shown.length - 2 && shown.length > 3 ? "up" : "down"}
                    summary={<Icon name="more" />}
                    summaryClassName="inline-flex size-8 items-center justify-center rounded-[9px] text-ink-soft hover:bg-paper"
                    panelClassName="min-w-52"
                  >
                    <button type="button" className={menuItem} onClick={() => setPanel(c)}><Icon name="edit" className="size-[15px] text-muted" />{t("edit")}</button>
                    <hr className="my-1 border-line-soft" />
                    {c.archived ? (
                      <button type="button" className={menuItem} onClick={() => restore(c)}><Icon name="archive" className="size-[15px] text-muted" />{t("restore")}</button>
                    ) : (
                      <button type="button" className={cx(menuItem, "text-danger")} onClick={() => setArchiving(c)}><Icon name="archive" className="size-[15px]" />{t("archive")}</button>
                    )}
                  </Menu>
                </td>
              </tr>
            ))}
            {shown.length === 0 && (
              <tr><td colSpan={6} className="px-3.5 py-8 text-center text-muted">{t("noMatch")}</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {panel && (
        <ClientPanel
          key={panel === "new" ? "new" : panel.id}
          client={panel === "new" ? null : panel}
          onClose={() => setPanel(null)}
          onSaved={() => {
            setError("");
            setPanel(null);
            router.refresh();
          }}
          onArchive={setArchiving}
          onRestore={restoreFromPanel}
        />
      )}
      {archiving && (
        <ConfirmDialog
          title={t("confirmArchive", { name: archiving.name })}
          body={t("confirmArchiveBody")}
          action={t("archive")}
          cancelLabel={t("cancel")}
          onConfirm={async () => {
            const problem = await setArchived(archiving, true);
            if (!problem) {
              setError("");
              setArchiving(null);
              setPanel(null);
            }
            return problem;
          }}
          onCancel={() => setArchiving(null)}
        />
      )}
    </div>
  );
}

type PanelProps = {
  client: Client | null; // null: a new client
  onClose: () => void;
  onSaved: () => void;
  onArchive: (c: Client) => void;
  onRestore: (c: Client) => Promise<string | undefined>;
};

function ClientPanel({ client, onClose, onSaved, onArchive, onRestore }: PanelProps) {
  const t = useTranslations("clients");
  const problemText = useProblemText();
  const [name, setName] = useState(client?.name ?? "");
  const [code, setCode] = useState(client?.code ?? "");
  const [aliases, setAliases] = useState<string[]>(client?.aliases ?? []);
  const [problem, setProblem] = useState<Problem>();
  const [restoreProblem, setRestoreProblem] = useState<string>();
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    // On update an empty code clears it; on create it is left out.
    const { error } = client
      ? await api.PATCH("/clients/{id}", { params: { path: { id: client.id } }, body: { name, code, aliases } })
      : await api.POST("/clients", { body: { name, code: code.trim() || undefined, aliases } });
    setBusy(false);
    if (error) return setProblem(error);
    onSaved();
  }

  return (
    <SidePanel
      labelledBy="client-panel-title"
      closeLabel={t("close")}
      onClose={onClose}
      title={
        <div className="flex min-w-0 flex-col gap-0.5">
          <h2 id="client-panel-title" className="truncate text-lg font-extrabold tracking-[-0.01em]">{client ? t("editTitle", { name: client.name }) : t("newClient")}</h2>
          {client && <span className="text-[12.5px] text-muted">{client.projects?.length ? t("usedIn", { projects: client.projects.join(", ") }) : t("notUsed")}</span>}
        </div>
      }
      footer={
        <>
          <button type="button" className={button.secondary} onClick={onClose}>{t("cancel")}</button>
          <button type="submit" form="panel-form" className={button.primary} disabled={busy}>{client ? t("save") : t("create")}</button>
        </>
      }
    >
      <form id="panel-form" onSubmit={submit} className="flex flex-col gap-4">
        <div className="grid grid-cols-[minmax(0,1fr)_7.5rem] gap-3">
          <label className={field.label}>
            {t("name")}
            <input value={name} onChange={(e) => setName(e.target.value)} required maxLength={200} className={field.input} />
          </label>
          <label className={field.label}>
            {t("code")}
            <input value={code} onChange={(e) => setCode(e.target.value)} maxLength={20} className={cx(field.input, "font-extrabold tracking-[0.02em]")} />
          </label>
        </div>
        <div className="flex flex-col gap-1.5">
          <label htmlFor="client-aliases" className="flex items-baseline text-[13px] font-semibold">
            {t("aliases")}
            <span className="ml-auto text-xs text-muted">{t("aliasCount", { count: aliases.length, max: maxAliases })}</span>
          </label>
          <AliasInput
            id="client-aliases"
            value={aliases}
            onChange={setAliases}
            labels={{ placeholder: t("aliasPlaceholder"), remove: (alias) => t("removeAlias", { alias }) }}
          />
          <span className={field.hint}>{t("aliasHelp")}</span>
        </div>
        {(problem || restoreProblem) && <p role="alert" className={field.error}>{problem ? problemText(problem) : restoreProblem}</p>}
        {client && (
          <div className="mt-2 flex items-center gap-3 border-t border-line-soft pt-4">
            <span className="flex flex-1 flex-col gap-0.5">
              <span className="text-[13.5px] font-bold">{client.archived ? t("restore") : t("archive")}</span>
              <span className="text-[12.5px] text-muted">{client.archived ? t("restoreHelp") : t("archiveHelp")}</span>
            </span>
            {client.archived ? (
              <button type="button" className={button.secondary} onClick={async () => setRestoreProblem(await onRestore(client))}>{t("restore")}</button>
            ) : (
              <button type="button" className={button.danger} onClick={() => onArchive(client)}>{t("archive")}</button>
            )}
          </div>
        )}
      </form>
    </SidePanel>
  );
}
