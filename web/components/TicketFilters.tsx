import { getTranslations } from "next-intl/server";
import Icon from "@/components/Icon";
import type { Client, Status } from "@/lib/problem";
import { button, field } from "@/lib/ui";

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
  const pick = "flex items-center gap-1.5 text-[13px] text-muted";
  return (
    <form method="get" action={action} aria-label={t("label")} className="flex flex-wrap items-center gap-2">
      <label className="flex h-8 items-center gap-1.5 rounded border border-line bg-white px-2 text-muted focus-within:outline-2 focus-within:outline-accent">
        <Icon name="search" />
        <input name="q" defaultValue={values.q} aria-label={t("q")} placeholder={t("qPlaceholder")} className="w-40 bg-transparent text-[13px] text-ink outline-none placeholder:text-muted" />
      </label>
      <label className={pick}>
        {t("client")}
        <select name="client" defaultValue={values.client ?? ""} className={field.compact}>
          <option value="">{t("allClients")}</option>
          <option value="core">{t("core")}</option>
          {clients.map((c) => (
            <option key={c.id} value={c.id}>{c.name}</option>
          ))}
        </select>
      </label>
      <label className={pick}>
        {t("type")}
        <select name="type" defaultValue={values.type ?? ""} className={field.compact}>
          <option value="">{t("anyType")}</option>
          {(["bug", "change_request", "feature"] as const).map((ty) => (
            <option key={ty} value={ty}>{tTypes(ty)}</option>
          ))}
        </select>
      </label>
      <label className={pick}>
        {t("assignee")}
        <select name="assignee" defaultValue={values.assignee ?? ""} className={field.compact}>
          <option value="">{t("anyone")}</option>
          <option value="me">{t("mine")}</option>
        </select>
      </label>
      {statuses && (
        <>
          <label className={pick}>
            {t("status")}
            <select name="status" defaultValue={values.status ?? ""} className={field.compact}>
              <option value="">{t("anyStatus")}</option>
              {statuses.map((s) => (
                <option key={s.id} value={s.id}>{s.name}</option>
              ))}
            </select>
          </label>
          <label className={pick}>
            {t("missing")}
            <select name="missing" defaultValue={values.missing ?? ""} className={field.compact}>
              <option value="">{t("nothingMissing")}</option>
              <option value="reason">{t("missingReason")}</option>
              <option value="menus">{t("missingMenus")}</option>
            </select>
          </label>
          <label className={pick}>
            {t("sort")}
            <select name="sort" defaultValue={values.sort ?? "updated"} className={field.compact}>
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
