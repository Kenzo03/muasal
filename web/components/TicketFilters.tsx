import { getTranslations } from "next-intl/server";
import { showsClients } from "@/components/Chips";
import Icon from "@/components/Icon";
import type { Client, Ref, Status } from "@/lib/problem";
import { button, cx } from "@/lib/ui";

type Props = {
  action: string;
  values: Record<string, string>;
  clients: Client[];
  assignees: Ref[]; // who can own tickets, so a lead can look at one person's work
  statuses?: Status[]; // the list filters by status; the board shows every status anyway
  labels?: string[]; // the project's labels, most used first (MSL-56)
};

// The filter bar of the board and the list (FSD §8.4, §8.5). It is a GET form,
// so every view is a URL people can share.
export default async function TicketFilters({ action, values, clients, assignees, statuses, labels = [] }: Props) {
  const t = await getTranslations("ticketFilters");
  const tTypes = await getTranslations("ticketTypes");
  // Each filter is a chip: its name, then a borderless select.
  const pick = "flex h-9 items-center gap-1 rounded-[10px] border border-line bg-white pl-3 text-[13.5px] font-semibold text-ink focus-within:border-accent";
  const select = "h-full cursor-pointer rounded-[10px] bg-transparent text-[13.5px] font-medium text-muted outline-none";
  const set = ["client", "type", "assignee", "label", "status", "due", "stale", "missing"].filter((k) => values[k]).length;
  return (
    <form method="get" action={action} aria-label={t("label")} className="flex flex-wrap items-center gap-2">
      <label className="flex h-9 items-center gap-2 rounded-[10px] border border-line bg-white px-3 text-muted focus-within:border-accent">
        <Icon name="search" />
        <input name="q" defaultValue={values.q} aria-label={t("q")} placeholder={t("qPlaceholder")} className="w-40 bg-transparent text-[13.5px] text-ink outline-none placeholder:text-muted" />
      </label>
      {/* MSL-35: on a phone the filters fold behind one button, open while any is set. */}
      <input id={`${action}-filters`} type="checkbox" defaultChecked={set > 0} className="peer sr-only md:hidden" />
      <label htmlFor={`${action}-filters`} className={cx(button.secondary, "cursor-pointer peer-focus-visible:outline-2 peer-focus-visible:outline-accent md:hidden")}>
        <Icon name="sliders" />
        {set > 0 ? t("filtersSet", { count: set }) : t("filters")}
      </label>
      <div className="hidden w-full flex-wrap items-center gap-2 peer-checked:flex md:flex md:w-auto">
        {showsClients(clients) && (
          <label htmlFor={`${action}-client`} className={pick}>
            {t("client")}
            <select id={`${action}-client`} name="client" defaultValue={values.client ?? ""} className={select}>
              <option value="">{t("allClients")}</option>
              <option value="core">{t("core")}</option>
              {clients.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          </label>
        )}
        <label htmlFor={`${action}-type`} className={pick}>
          {t("type")}
          <select id={`${action}-type`} name="type" defaultValue={values.type ?? ""} className={select}>
            <option value="">{t("anyType")}</option>
            {(["bug", "change_request", "feature"] as const).map((ty) => (
              <option key={ty} value={ty}>{tTypes(ty)}</option>
            ))}
          </select>
        </label>
        <label htmlFor={`${action}-assignee`} className={pick}>
          {t("assignee")}
          <select id={`${action}-assignee`} name="assignee" defaultValue={values.assignee ?? ""} className={select}>
            <option value="">{t("anyone")}</option>
            <option value="me">{t("mine")}</option>
            <option value="none">{t("unassigned")}</option>
            {assignees.map((a) => (
              <option key={a.id} value={a.id}>{a.name}</option>
            ))}
          </select>
        </label>
        {labels.length > 0 && (
          <label htmlFor={`${action}-label`} className={pick}>
            {t("label")}
            <select id={`${action}-label`} name="label" defaultValue={values.label ?? ""} className={select}>
              <option value="">{t("anyLabel")}</option>
              {labels.map((l) => (
                <option key={l} value={l}>{l}</option>
              ))}
            </select>
          </label>
        )}
        {statuses && (
          <>
            <label htmlFor={`${action}-status`} className={pick}>
              {t("status")}
              <select id={`${action}-status`} name="status" defaultValue={values.status ?? ""} className={select}>
                <option value="">{t("anyStatus")}</option>
                <option value="open">{t("openStatuses")}</option>
                {statuses.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
            </label>
            <label htmlFor={`${action}-due`} className={pick}>
              {t("due")}
              <select id={`${action}-due`} name="due" defaultValue={values.due ?? ""} className={select}>
                <option value="">{t("anyDue")}</option>
                <option value="overdue">{t("overdue")}</option>
                <option value="week">{t("dueWeek")}</option>
              </select>
            </label>
            <label htmlFor={`${action}-accepted`} className={pick}>
              {t("accepted")}
              <select id={`${action}-accepted`} name="accepted" defaultValue={values.accepted ?? ""} className={select}>
                <option value="">{t("anyAcceptance")}</option>
                <option value="yes">{t("acceptedYes")}</option>
                <option value="no">{t("acceptedNo")}</option>
              </select>
            </label>
            <label htmlFor={`${action}-stale`} className={pick}>
              {t("stale")}
              <select id={`${action}-stale`} name="stale" defaultValue={values.stale ?? ""} className={select}>
                <option value="">{t("anyActivity")}</option>
                {/* A link from the workload page may carry another number of days. */}
                {[...new Set([7, 14, 30, ...(/^\d+$/.test(values.stale ?? "") ? [Number(values.stale)] : [])])]
                  .sort((a, b) => a - b)
                  .map((d) => (
                    <option key={d} value={d}>{t("staleDays", { days: d })}</option>
                  ))}
              </select>
            </label>
            <label htmlFor={`${action}-missing`} className={pick}>
              {t("missing")}
              <select id={`${action}-missing`} name="missing" defaultValue={values.missing ?? ""} className={select}>
                <option value="">{t("nothingMissing")}</option>
                <option value="reason">{t("missingReason")}</option>
                <option value="menus">{t("missingMenus")}</option>
                <option value="weak_reason">{t("weakReason")}</option>
              </select>
            </label>
            <label htmlFor={`${action}-sort`} className={pick}>
              {t("sort")}
              <select id={`${action}-sort`} name="sort" defaultValue={values.sort ?? "updated"} className={select}>
                <option value="updated">{t("sortUpdated")}</option>
                <option value="created">{t("sortCreated")}</option>
                <option value="key">{t("sortKey")}</option>
                <option value="priority">{t("sortPriority")}</option>
                <option value="due">{t("sortDue")}</option>
              </select>
            </label>
          </>
        )}
        <button className={button.secondary}>{t("apply")}</button>
        <a href={action} className={button.quiet}>{t("reset")}</a>
      </div>
    </form>
  );
}
