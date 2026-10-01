"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTimeZone, useTranslations } from "next-intl";
import { Avatar } from "@/components/Chips";
import ConfirmDialog from "@/components/ConfirmDialog";
import FilterTabs from "@/components/FilterTabs";
import Icon from "@/components/Icon";
import Menu from "@/components/Menu";
import SidePanel from "@/components/SidePanel";
import { matches, userStatus, type UserStatus } from "@/lib/admin";
import { api } from "@/lib/api";
import { dateTime } from "@/lib/format";
import { useProblemText, type Problem, type User } from "@/lib/problem";
import { button, chip, cx, field, table } from "@/lib/ui";

type Filter = "all" | UserStatus;
type Confirm = { kind: "reset" | "link" | "disable"; user: User };

const statusTone: Record<UserStatus, string> = {
  active: "bg-ok-soft text-ok",
  invited: "bg-warn-soft text-warn",
  disabled: "bg-well text-muted",
};
const menuItem = "flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-ink hover:bg-paper";

export default function UsersAdmin({ users, meId }: { users: User[]; meId: number }) {
  const t = useTranslations("users");
  const problemText = useProblemText();
  const locale = useLocale();
  const timeZone = useTimeZone();
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [panel, setPanel] = useState<User | "new" | null>(null);
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [error, setError] = useState("");
  // MSL-20: every link made here stays listed until dismissed, newest first.
  const [links, setLinks] = useState<{ name: string; url: string }[]>([]);
  const [copied, setCopied] = useState("");
  const addLink = (name: string, url: string) => setLinks((ls) => [{ name, url }, ...ls.filter((l) => l.name !== name)]);

  const counts = useMemo(() => {
    const c = { all: users.length, active: 0, invited: 0, disabled: 0 };
    for (const u of users) c[userStatus(u)]++;
    return c;
  }, [users]);
  const shown = users.filter((u) => (filter === "all" || userStatus(u) === filter) && matches(query, u.name, u.email));
  const lastLogin = (u: User) => (u.last_login_at ? dateTime(u.last_login_at, locale, timeZone) : t("never"));

  async function setDisabled(u: User, disabled: boolean): Promise<string | undefined> {
    const { error } = await api.PATCH("/admin/users/{id}", { params: { path: { id: u.id } }, body: { disabled } });
    if (error) return problemText(error);
    router.refresh();
  }
  async function newLink(u: User): Promise<string | undefined> {
    const { data, error } = await api.POST("/admin/users/{id}/setup-link", { params: { path: { id: u.id } } });
    if (error) return problemText(error);
    addLink(u.name, data.url);
    router.refresh();
  }
  async function runConfirm(c: Confirm) {
    const problem = c.kind === "disable" ? await setDisabled(c.user, true) : await newLink(c.user);
    if (!problem) {
      setConfirm(null);
      setPanel(null);
    }
    return problem;
  }
  // Enable needs no confirmation; a failure shows in the page's alert line.
  async function enable(u: User) {
    const problem = await setDisabled(u, false);
    setError(problem ?? "");
  }
  const ask = (u: User, kind: Confirm["kind"]) => setConfirm({ user: u, kind });
  const askLink = (u: User) => ask(u, u.has_password ? "reset" : "link");

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
          options={(["all", "active", "invited", "disabled"] as const).map((k) => ({ key: k, label: t(k), count: counts[k] }))}
        />
        <button type="button" className={cx(button.primary, "ml-auto")} onClick={() => setPanel("new")}>
          <Icon name="plus" />
          {t("newUser")}
        </button>
      </div>
      {error && <p role="alert" className={field.error}>{error}</p>}
      {links.map((link) => (
        <p key={link.url} role="status" className="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-xl border border-warn-line bg-warn-soft px-3.5 py-2.5 text-[13.5px] text-warn">
          {t("linkFor", { name: link.name })}
          <code data-testid="setup-link" className="min-w-0 flex-1 break-all font-mono text-ink">{link.url}</code>
          <button type="button" className={button.quiet} onClick={() => navigator.clipboard?.writeText(link.url).then(() => setCopied(link.url), () => {})}>
            {copied === link.url ? t("copied") : t("copy")}
          </button>
          <button type="button" aria-label={t("dismiss")} className={button.quiet} onClick={() => setLinks((ls) => ls.filter((l) => l.url !== link.url))}>
            <Icon name="x" className="size-3.5" />
          </button>
        </p>
      ))}
      {/* md:overflow-visible: the row menu would be clipped by the wrapper's scroll box; phones keep the sideways scroll. */}
      <div className={cx(table.wrap, "md:overflow-visible")}>
        <table className={table.table}>
          <thead className={table.head}>
            <tr>
              <th className={table.th}>{t("name")}</th>
              <th className={table.th}>{t("email")}</th>
              <th className={table.th}>{t("status")}</th>
              <th className={table.th}>{t("lastLogin")}</th>
              <th className={cx(table.th, "w-14")}><span className="sr-only">{t("actionsFor", { name: "" })}</span></th>
            </tr>
          </thead>
          <tbody>
            {shown.map((u, i) => {
              const status = userStatus(u);
              const me = u.id === meId;
              return (
                <tr key={u.id} className={cx(table.row, panel !== "new" && panel?.id === u.id && "bg-accent-soft/40")}>
                  <td className={table.td}>
                    <button type="button" onClick={() => setPanel(u)} className="flex max-w-full items-center gap-2.5 text-left font-bold text-ink hover:text-accent-strong">
                      <Avatar name={u.name} className="size-7 bg-[#F6D9CC] text-[#8A3417] text-[11px]" />
                      <span className="truncate">{u.name}</span>
                      {u.is_admin && <span className={cx(chip, "bg-accent-soft text-accent-strong")}>{t("admin")}</span>}
                      {me && <span className="font-medium text-muted">{t("you")}</span>}
                    </button>
                  </td>
                  <td className={cx(table.td, "text-ink-soft")}>{u.email}</td>
                  <td className={table.td}><span className={cx(chip, statusTone[status])}>{t(status)}</span></td>
                  <td className={cx(table.td, "whitespace-nowrap text-muted")}>{lastLogin(u)}</td>
                  <td className={cx(table.td, "py-1.5 text-right")}>
                    {!me && (
                      <Menu
                        label={t("actionsFor", { name: u.name })}
                        align="right"
                        side={i >= shown.length - 2 && shown.length > 3 ? "up" : "down"}
                        summary={<Icon name="more" />}
                        summaryClassName="inline-flex size-8 items-center justify-center rounded-[9px] text-ink-soft hover:bg-paper"
                        panelClassName="min-w-56"
                      >
                        <button type="button" className={menuItem} onClick={() => setPanel(u)}><Icon name="edit" className="size-[15px] text-muted" />{t("edit")}</button>
                        <button type="button" className={menuItem} onClick={() => askLink(u)}><Icon name="lock" className="size-[15px] text-muted" />{u.has_password ? t("resetPassword") : t("newLink")}</button>
                        <hr className="my-1 border-line-soft" />
                        {u.disabled ? (
                          <button type="button" className={menuItem} onClick={() => enable(u)}><Icon name="check" className="size-[15px] text-muted" />{t("enable")}</button>
                        ) : (
                          <button type="button" className={cx(menuItem, "text-danger")} onClick={() => ask(u, "disable")}><Icon name="xCircle" className="size-[15px]" />{t("disable")}</button>
                        )}
                      </Menu>
                    )}
                  </td>
                </tr>
              );
            })}
            {shown.length === 0 && (
              <tr><td colSpan={5} className="px-3.5 py-8 text-center text-muted">{t("noMatch")}</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {panel && (
        <UserPanel
          key={panel === "new" ? "new" : panel.id}
          user={panel === "new" ? null : panel}
          isMe={panel !== "new" && panel.id === meId}
          lastLogin={panel === "new" ? "" : lastLogin(panel)}
          onClose={() => setPanel(null)}
          onCreated={(name, url) => {
            addLink(name, url);
            setPanel(null);
            router.refresh();
          }}
          onSaved={() => {
            setPanel(null);
            router.refresh();
          }}
          onLink={askLink}
          onDisable={(u) => ask(u, "disable")}
          onEnable={enable}
        />
      )}
      {confirm && (
        <ConfirmDialog
          title={t(confirm.kind === "reset" ? "confirmReset" : confirm.kind === "link" ? "confirmLink" : "confirmDisable", { name: confirm.user.name })}
          body={t(confirm.kind === "reset" ? "confirmResetBody" : confirm.kind === "link" ? "confirmLinkBody" : "confirmDisableBody")}
          action={t(confirm.kind === "reset" ? "resetPassword" : confirm.kind === "link" ? "newLink" : "disable")}
          cancelLabel={t("cancel")}
          onConfirm={() => runConfirm(confirm)}
          onCancel={() => setConfirm(null)}
        />
      )}
    </div>
  );
}

type PanelProps = {
  user: User | null; // null: a new user
  isMe: boolean;
  lastLogin: string;
  onClose: () => void;
  onCreated: (name: string, url: string) => void;
  onSaved: () => void;
  onLink: (u: User) => void;
  onDisable: (u: User) => void;
  onEnable: (u: User) => void;
};

function UserPanel({ user, isMe, lastLogin, onClose, onCreated, onSaved, onLink, onDisable, onEnable }: PanelProps) {
  const t = useTranslations("users");
  const problemText = useProblemText();
  const [name, setName] = useState(user?.name ?? "");
  const [email, setEmail] = useState("");
  const [admin, setAdmin] = useState(user?.is_admin ?? false);
  const [problem, setProblem] = useState<Problem>();
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    if (!user) {
      const { data, error } = await api.POST("/admin/users", { body: { name, email, is_admin: admin } });
      setBusy(false);
      if (error) return setProblem(error);
      return onCreated(data.user.name, data.setup_link.url);
    }
    // Only the fields that changed, so a save never undoes someone else's edit.
    const body: { name?: string; is_admin?: boolean } = {};
    if (name !== user.name) body.name = name;
    if (admin !== user.is_admin) body.is_admin = admin;
    if (Object.keys(body).length === 0) return onClose();
    const { error } = await api.PATCH("/admin/users/{id}", { params: { path: { id: user.id } }, body });
    setBusy(false);
    if (error) return setProblem(error);
    onSaved();
  }

  const status = user ? userStatus(user) : null;
  return (
    <SidePanel
      labelledBy="user-panel-title"
      closeLabel={t("close")}
      onClose={onClose}
      title={
        user ? (
          <div className="flex items-center gap-3">
            <Avatar name={user.name} className="size-10 bg-[#F6D9CC] text-[#8A3417] text-sm" />
            <div className="flex min-w-0 flex-col gap-0.5">
              <h2 id="user-panel-title" className="truncate text-lg font-extrabold tracking-[-0.01em]">{user.name}</h2>
              <span className="flex items-center gap-2 text-[12.5px] text-muted">
                {status && <span className={cx(chip, statusTone[status])}>{t(status)}</span>}
                {t("lastLoginAt", { when: lastLogin })}
              </span>
            </div>
          </div>
        ) : (
          <h2 id="user-panel-title" className="text-lg font-extrabold tracking-[-0.01em]">{t("newUser")}</h2>
        )
      }
      footer={
        <>
          <button type="button" className={button.secondary} onClick={onClose}>{t("cancel")}</button>
          <button type="submit" form="panel-form" className={button.primary} disabled={busy}>{user ? t("save") : t("create")}</button>
        </>
      }
    >
      <form id="panel-form" onSubmit={submit} className="flex flex-col gap-4">
        <label className={field.label}>
          {t("name")}
          <input value={name} onChange={(e) => setName(e.target.value)} required maxLength={200} className={field.input} />
        </label>
        <div className="flex flex-col gap-1.5">
          <label className={field.label}>
            {t("email")}
            {user ? (
              <input value={user.email} readOnly aria-describedby="email-help" className={cx(field.input, "bg-well text-ink-soft")} />
            ) : (
              <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required aria-describedby="email-help" className={field.input} />
            )}
          </label>
          <span id="email-help" className={cx(field.hint, "font-normal")}>{user ? t("emailFixed") : t("newHelp")}</span>
        </div>
        <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-line bg-paper px-3.5 py-3">
          <input type="checkbox" checked={admin} disabled={isMe} onChange={(e) => setAdmin(e.target.checked)} className="mt-0.5 size-4 accent-accent" />
          <span className="flex flex-col gap-0.5">
            <span className="text-sm font-bold">{t("systemAdmin")}</span>
            <span className="text-[12.5px] text-muted">{t("systemAdminHelp")}</span>
          </span>
        </label>
        {problem && <p role="alert" className={field.error}>{problemText(problem)}</p>}
        {user && !isMe && (
          <section aria-labelledby="access-title" className="mt-2 flex flex-col gap-3 border-t border-line-soft pt-4">
            <h3 id="access-title" className="text-[13px] font-bold text-muted">{t("access")}</h3>
            <div className="flex items-center gap-3">
              <span className="flex flex-1 flex-col gap-0.5">
                <span className="text-[13.5px] font-bold">{user.has_password ? t("resetPassword") : t("newLink")}</span>
                <span className="text-[12.5px] text-muted">{user.has_password ? t("resetHelp") : t("newLinkHelp")}</span>
              </span>
              <button type="button" className={button.secondary} onClick={() => onLink(user)}>{user.has_password ? t("resetPassword") : t("newLink")}</button>
            </div>
            <div className="flex items-center gap-3">
              <span className="flex flex-1 flex-col gap-0.5">
                <span className="text-[13.5px] font-bold">{user.disabled ? t("enable") : t("disable")}</span>
                <span className="text-[12.5px] text-muted">{user.disabled ? t("enableHelp") : t("disableHelp")}</span>
              </span>
              {user.disabled ? (
                <button type="button" className={button.secondary} onClick={() => onEnable(user)}>{t("enable")}</button>
              ) : (
                <button type="button" className={button.danger} onClick={() => onDisable(user)}>{t("disable")}</button>
              )}
            </div>
          </section>
        )}
      </form>
    </SidePanel>
  );
}
