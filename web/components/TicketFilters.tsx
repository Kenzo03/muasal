import { getTranslations } from "next-intl/server";
import { showsClients } from "@/components/Chips";
import Icon from "@/components/Icon";
import type { Client, Status } from "@/lib/problem";
import { button } from "@/lib/ui";

type Props = {
  action: string;
  values: Record<string, string>;
  clients: Client[];
  statuses?: Status[]; // the list filters by status; the board shows every status anyway
};

// The filter bar of the board and the list (FSD §8.4, §8.5). It is a GET form,
// so every view is a URL people can share.
export default async function TicketFilters({ action, values, clients, statuses }: Props) {
  const t = await getTranslations("ticketFilters");
  const tTypes = await getTranslations("ticketTypes");
  // Each filter is a chip: its name, then a borderless select.
  const pick = "flex h-9 items-center gap-1 rounded-[10px] border border-line bg-white pl-3 text-[13.5px] font-semibold text-ink focus-within:border-accent";
  const select = "h-full cursor-pointer rounded-[10px] bg-transparent text-[13.5px] font-medium text-muted outline-none";
  return (
    <form method="get" action={action} aria-label={t("label")} className="flex flex-wrap items-center gap-2">
      <label className="flex h-9 items-center gap-2 rounded-[10px] border border-line bg-white px-3 text-muted focus-within:border-accent">
        <Icon name="search" />
        <input name="q" defaultValue={values.q} aria-label={t("q")} placeholder={t("qPlaceholder")} className="w-40 bg-transparent text-[13.5px] text-ink outline-none placeholder:text-muted" />
      </label>
      {showsClients(clients) && (
        <label className={pick}>
          {t("client")}
          <select name="client" defaultValue={values.client ?? ""} className={select}>
            <option value="">{t("allClients")}</option>
            <option value="core">{t("core")}</option>
            {clients.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
        </label>
      )}
      <label className={pick}>
        {t("type")}
        <select name="type" defaultValue={values.type ?? ""} className={select}>
          <option value="">{t("anyType")}</option>
          {(["bug", "change_request", "feature"] as const).map((ty) => (
            <option key={ty} value={ty}>{tTypes(ty)}</option>
          ))}
        </select>
      </label>
      <label className={pick}>
        {t("assignee")}
        <select name="assignee" defaultValue={values.assignee ?? ""} className={select}>
          <option value="">{t("anyone")}</option>
          <option value="me">{t("mine")}</option>
        </select>
      </label>
      {statuses && (
        <>
          <label className={pick}>
            {t("status")}
            <select name="status" defaultValue={values.status ?? ""} className={select}>
              <option value="">{t("anyStatus")}</option>
              {statuses.map((s) => (
                <option key={s.id} value={s.id}>{s.name}</option>
              ))}
            </select>
          </label>
          <label className={pick}>
            {t("missing")}
            <select name="missing" defaultValue={values.missing ?? ""} className={select}>
              <option value="">{t("nothingMissing")}</option>
              <option value="reason">{t("missingReason")}</option>
              <option value="menus">{t("missingMenus")}</option>
            </select>
          </label>
          <label className={pick}>
            {t("sort")}
            <select name="sort" defaultValue={values.sort ?? "updated"} className={select}>
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
    </form>
  );
}
