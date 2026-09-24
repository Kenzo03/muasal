import { getTranslations } from "next-intl/server";
import type { Client, Status } from "@/lib/problem";

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
  const input = "rounded border px-2 py-1";
  return (
    <form method="get" action={action} aria-label={t("label")} className="flex flex-wrap items-end gap-3 text-sm">
      <label className="flex flex-col gap-1">
        {t("q")}
        <input name="q" defaultValue={values.q} placeholder={t("qPlaceholder")} className={input} />
      </label>
      <label className="flex flex-col gap-1">
        {t("client")}
        <select name="client" defaultValue={values.client ?? ""} className={input}>
          <option value="">{t("allClients")}</option>
          <option value="core">{t("core")}</option>
          {clients.map((c) => (
            <option key={c.id} value={c.id}>{c.name}</option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1">
        {t("type")}
        <select name="type" defaultValue={values.type ?? ""} className={input}>
          <option value="">{t("anyType")}</option>
          {(["bug", "change_request", "feature"] as const).map((ty) => (
            <option key={ty} value={ty}>{tTypes(ty)}</option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1">
        {t("assignee")}
        <select name="assignee" defaultValue={values.assignee ?? ""} className={input}>
          <option value="">{t("anyone")}</option>
          <option value="me">{t("mine")}</option>
        </select>
      </label>
      {statuses && (
        <>
          <label className="flex flex-col gap-1">
            {t("status")}
            <select name="status" defaultValue={values.status ?? ""} className={input}>
              <option value="">{t("anyStatus")}</option>
              {statuses.map((s) => (
                <option key={s.id} value={s.id}>{s.name}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1">
            {t("missing")}
            <select name="missing" defaultValue={values.missing ?? ""} className={input}>
              <option value="">{t("nothingMissing")}</option>
              <option value="reason">{t("missingReason")}</option>
              <option value="menus">{t("missingMenus")}</option>
            </select>
          </label>
          <label className="flex flex-col gap-1">
            {t("sort")}
            <select name="sort" defaultValue={values.sort ?? "updated"} className={input}>
              <option value="updated">{t("sortUpdated")}</option>
              <option value="created">{t("sortCreated")}</option>
              <option value="key">{t("sortKey")}</option>
              <option value="priority">{t("sortPriority")}</option>
              <option value="due">{t("sortDue")}</option>
            </select>
          </label>
        </>
      )}
      <button className="rounded bg-neutral-900 px-3 py-1 text-white">{t("apply")}</button>
      <a href={action} className="underline">{t("reset")}</a>
    </form>
  );
}
