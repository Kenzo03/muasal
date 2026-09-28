"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Avatar, initials } from "@/components/Chips";
import Icon, { type IconName } from "@/components/Icon";
import Menu from "@/components/Menu";
import { api } from "@/lib/api";
import type { Project, User } from "@/lib/problem";
import { cx } from "@/lib/ui";
import SignOutButton from "./SignOutButton";

type Props = {
  me: User;
  projects: Project[];
  project?: Project;
  rail: boolean; // icons only, from md up
  drawer: boolean; // open over the page on a phone
  onNavigate: () => void;
  onToggleRail: () => void;
};

type Page = [href: string, icon: IconName, label: string];

const menuItem = "block px-3.5 py-2 text-sm text-ink no-underline hover:bg-paper hover:text-ink";

// The sidebar (FSD §6.1): Home and Ask, then the pages of the project in view.
// Outside a project it lists the user's projects and, for system admins, the
// admin pages. The switcher on top shows a long project name on two lines; its
// menu shows the name whole.
export default function Sidebar({ me, projects, project, rail, drawer, onNavigate, onToggleRail }: Props) {
  const t = useTranslations("nav");
  const tp = useTranslations("project");
  const path = usePathname();
  const router = useRouter();

  async function setLocale(locale: "id" | "en") {
    if (locale === me.locale) return;
    const { error } = await api.PATCH("/me", { body: { locale } });
    if (error) return;
    document.cookie = `locale=${locale}; path=/; max-age=31536000; samesite=lax`;
    router.refresh();
  }

  const projectPages: Page[] = project
    ? [
        [`/p/${project.key}/board`, "board", tp("board")],
        [`/p/${project.key}/tickets`, "list", tp("tickets")],
        [`/p/${project.key}/workload`, "users", tp("workload")],
        [`/p/${project.key}/modules`, "tree", tp("modules")],
        [`/p/${project.key}/notes`, "notes", tp("notes")],
        [`/p/${project.key}/documents`, "file", tp("documents")],
        [`/p/${project.key}/summaries`, "summary", tp("summaries")],
        ...(project.role === "admin" ? [[`/p/${project.key}/settings`, "sliders", tp("settings")] as Page] : []),
      ]
    : [];
  const adminPages: Page[] = [
    ["/admin/users", "users", t("users")],
    ["/admin/clients", "building", t("clients")],
    ["/admin/ai", "sparkle", t("ai")],
    ["/admin/ask-log", "list", t("askLog")],
    ["/admin/audit", "file", t("audit")],
    ["/admin/imports", "upload", t("imports")],
    ["/admin/backups", "archive", t("backups")],
    ["/admin/system", "pulse", t("system")],
  ];
  const isActive = (href: string) => path.startsWith(href) || (href.endsWith("/tickets") && path.startsWith("/t/"));
  // In the rail, labels stay for screen readers and show as tooltips.
  const hide = rail ? "md:sr-only" : "";

  const itemClass = (active: boolean) =>
    cx(
      "flex h-9 items-center gap-2.5 rounded-[10px] px-2.5 text-sm no-underline",
      active
        ? "bg-white font-bold text-ink shadow-[0_1px_2px_rgba(43,36,32,0.08),0_0_0_1px_rgba(43,36,32,0.05)] hover:text-ink"
        : "font-medium text-ink-soft hover:bg-white/70 hover:text-ink",
      rail && "md:justify-center md:px-0",
    );
  const item = ([href, icon, label]: Page, active: boolean) => (
    <li key={href}>
      <Link href={href} aria-current={active ? "page" : undefined} title={rail ? label : undefined} className={itemClass(active)}>
        <Icon name={icon} className={cx("size-4", active ? "text-accent" : "text-muted")} />
        <span className={cx("truncate", hide)}>{label}</span>
      </Link>
    </li>
  );
  const group = (label: string, children: React.ReactNode) => (
    <div className="flex flex-col">
      <h2 className={cx("px-2.5 pb-1.5 text-xs font-bold text-muted", hide)}>{label}</h2>
      {rail && <span aria-hidden="true" className="mx-auto mb-2 hidden h-px w-6 bg-field md:block" />}
      <ul className="flex flex-col gap-0.5">{children}</ul>
    </div>
  );
  const tile = (name: string, size: string) => (
    <span className={cx("flex shrink-0 items-center justify-center rounded-lg bg-accent-soft font-extrabold text-accent-strong", size)}>{initials(name)}</span>
  );

  return (
    <aside
      id="sidebar"
      onClick={(e) => {
        if ((e.target as HTMLElement).closest("a")) onNavigate();
      }}
      className={cx(
        "fixed inset-y-0 left-0 z-40 flex w-62 flex-col gap-4 border-r border-line bg-sidebar px-3 py-4 transition-transform print:hidden",
        "md:sticky md:top-0 md:z-20 md:h-screen md:translate-x-0 md:transition-none",
        drawer ? "translate-x-0" : "-translate-x-full",
        rail && "md:w-16 md:px-2",
      )}
    >
      <Link href="/" className={cx("flex items-center gap-2.5 px-1.5 text-ink no-underline hover:text-ink", rail && "md:justify-center md:px-0")}>
        <span className="flex size-8 shrink-0 items-center justify-center rounded-[9px] bg-accent text-white">
          <Icon name="logo" className="size-[18px]" />
        </span>
        <span className={cx("text-[17px] font-extrabold tracking-[-0.015em]", hide)}>Muasal</span>
      </Link>

      <Menu
        label={t("switchProject")}
        panelClassName="w-80"
        summaryClassName={cx(
          "flex w-full items-center gap-2.5 rounded-xl border border-line bg-white p-2 text-left shadow-[0_1px_2px_rgba(43,36,32,0.04)] hover:border-field",
          rail && "md:justify-center md:p-1.5",
        )}
        summary={
          <>
            {project ? (
              tile(project.name, "size-8 text-[11px]")
            ) : (
              <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-well text-muted">
                <Icon name="board" />
              </span>
            )}
            <span className={cx("min-w-0 flex-1", hide)}>
              <span className="line-clamp-2 text-[13.5px] font-bold leading-snug text-ink">{project?.name ?? t("pickProject")}</span>
              {project && <span className="block text-xs text-muted">{project.key}</span>}
            </span>
            <Icon name="updown" className={cx("size-4 text-muted", rail && "md:hidden")} />
          </>
        }
      >
        {project && (
          <div className="-mt-1.5 border-b border-line-soft bg-paper px-3.5 pb-3 pt-3">
            <p className="text-xs font-bold text-muted">
              {t("currentProject")} · {project.key}
            </p>
            <p className="mt-1 text-sm font-bold leading-snug">{project.name}</p>
          </div>
        )}
        <ul className="max-h-80 overflow-y-auto py-1">
          {projects.map((p) => (
            <li key={p.id}>
              <Link href={`/p/${p.key}/board`} className="flex items-center gap-2.5 px-3 py-2 text-ink no-underline hover:bg-paper hover:text-ink">
                {tile(p.name, "size-7 text-[10.5px]")}
                <span className="min-w-0">
                  <span className="line-clamp-2 text-[13.5px] font-semibold leading-snug">{p.name}</span>
                  <span className="block text-xs text-muted">{p.key}</span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
        <Link href="/" className="block border-t border-line-soft px-3.5 pb-1 pt-2.5 text-[13px] font-semibold no-underline">
          {t("allProjects")}
        </Link>
      </Menu>

      <nav aria-label={t("label")} className="-mx-1 flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-1">
        <ul className="flex flex-col gap-0.5">
          {item(["/", "home", t("home")], path === "/")}
          {item(["/ask", "sparkle", t("ask")], path.startsWith("/ask"))}
        </ul>
        {project ? (
          group(tp("nav"), projectPages.map((page) => item(page, isActive(page[0]))))
        ) : (
          <>
            {projects.length > 0 &&
              group(
                t("projects"),
                projects.map((p) => (
                  <li key={p.id}>
                    <Link href={`/p/${p.key}/board`} title={rail ? p.name : undefined} className={itemClass(false)}>
                      {tile(p.name, "size-5 rounded-md text-[9px]")}
                      <span className={cx("truncate", hide)}>{p.name}</span>
                    </Link>
                  </li>
                )),
              )}
            {me.is_admin && group(t("admin"), adminPages.map((page) => item(page, isActive(page[0]))))}
          </>
        )}
      </nav>

      <div className="flex flex-col gap-1 border-t border-line pt-3">
        {project && me.is_admin && <ul>{item(["/admin/users", "shield", t("admin")], path.startsWith("/admin"))}</ul>}
        <Menu
          side="up"
          label={t("account", { name: me.name })}
          panelClassName="w-64"
          summaryClassName={cx("flex w-full items-center gap-2.5 rounded-[10px] p-1.5 text-left hover:bg-white/70", rail && "md:justify-center")}
          summary={
            <>
              <Avatar name={me.name} className="size-8 bg-accent-soft text-xs font-bold text-accent-strong" />
              <span className={cx("min-w-0 flex-1", hide)}>
                <span className="block truncate text-[13.5px] font-bold text-ink">{me.name}</span>
                <span className="block truncate text-xs text-muted">{me.email}</span>
              </span>
            </>
          }
        >
          <Link href="/settings/profile" className={menuItem}>{t("profile")}</Link>
          <Link href="/settings/tokens" className={menuItem}>{t("tokens")}</Link>
          <div className="flex items-center justify-between gap-3 px-3.5 py-2 text-sm">
            <span id="language-label">{t("language")}</span>
            <div role="group" aria-labelledby="language-label" className="flex overflow-hidden rounded-lg border border-line text-xs font-bold">
              {(["id", "en"] as const).map((l) => (
                <button
                  key={l}
                  type="button"
                  aria-pressed={me.locale === l}
                  onClick={() => setLocale(l)}
                  className={cx("cursor-pointer px-2.5 py-1 uppercase", me.locale === l ? "bg-accent-soft text-accent-strong" : "text-muted hover:text-ink")}
                >
                  {l}
                </button>
              ))}
            </div>
          </div>
          <SignOutButton label={t("signOut")} />
        </Menu>
        <button
          type="button"
          onClick={onToggleRail}
          aria-label={rail ? t("expand") : undefined}
          title={rail ? t("expand") : undefined}
          className={cx(
            "hidden h-9 cursor-pointer items-center gap-2.5 rounded-[10px] px-2.5 text-[13px] font-semibold text-muted hover:bg-white/70 hover:text-ink md:flex",
            rail && "md:justify-center md:px-0",
          )}
        >
          <Icon name={rail ? "expand" : "collapse"} />
          {!rail && t("collapse")}
        </button>
      </div>
    </aside>
  );
}
