"use client";

import Link from "next/link";
import Form from "next/form";
import { usePathname, useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Avatar } from "@/components/Chips";
import Icon from "@/components/Icon";
import Menu from "@/components/Menu";
import { api } from "@/lib/api";
import type { Project, User } from "@/lib/problem";
import { button, cx } from "@/lib/ui";
import SignOutButton from "./SignOutButton";

// The project a page belongs to: /p/{key}/… or a ticket page such as /t/HRIS-231.
function currentKey(path: string): string | undefined {
  const m = path.match(/^\/p\/([^/]+)/) ?? path.match(/^\/t\/(.+)-\d+$/);
  return m?.[1].toUpperCase();
}

// The dark top bar of the Terakota design: the project switcher and the
// project's tabs, search, New ticket, the language switch and the account menu.
// Off project pages, system admins get their admin pages as tabs instead.
export default function TopBar({ me, projects }: { me: User; projects: Project[] }) {
  const t = useTranslations("nav");
  const tp = useTranslations("project");
  const path = usePathname();
  const router = useRouter();
  const project = projects.find((p) => p.key === currentKey(path));

  async function setLocale(locale: "id" | "en") {
    if (locale === me.locale) return;
    const { error } = await api.PATCH("/me", { body: { locale } });
    if (error) return;
    document.cookie = `locale=${locale}; path=/; max-age=31536000; samesite=lax`;
    router.refresh();
  }

  const tabs: [string, string][] = project
    ? [
        [`/p/${project.key}/board`, tp("board")],
        [`/p/${project.key}/tickets`, tp("tickets")],
        [`/p/${project.key}/modules`, tp("modules")],
        ...(project.role === "admin" ? [[`/p/${project.key}/settings`, tp("settings")] as [string, string]] : []),
      ]
    : me.is_admin
      ? [
          ["/admin/users", t("users")],
          ["/admin/clients", t("clients")],
        ]
      : [];
  const isActive = (href: string) => path.startsWith(href) || (href.endsWith("/tickets") && path.startsWith("/t/"));

  return (
    <header className="bg-bar text-white">
      <div className="flex min-h-13 flex-wrap items-center gap-x-4 px-4 md:px-5">
        <Link href="/" className="flex h-13 items-center gap-2 text-white no-underline hover:text-white">
          <Icon name="logo" className="size-5 text-bar-accent" />
          <span className="text-base font-semibold tracking-[0.01em]">Muasal</span>
        </Link>
        <Menu
          label={t("switchProject")}
          summaryClassName="flex h-8 items-center gap-2 rounded border border-bar-line bg-bar-raised px-2.5 text-[13px] text-white"
          summary={
            <>
              <span className={project ? "font-mono font-semibold" : ""}>{project?.key ?? t("projects")}</span>
              <Icon name="chevron" className="size-4 text-bar-muted" />
            </>
          }
        >
          {projects.map((p) => (
            <Link key={p.id} href={`/p/${p.key}/board`} className="flex items-baseline gap-2 px-3 py-2 text-sm text-ink no-underline hover:bg-paper hover:text-ink">
              <span className="font-mono text-xs font-semibold text-muted">{p.key}</span>
              {p.name}
            </Link>
          ))}
          <Link href="/" className="block border-t border-line-soft px-3 py-2 text-sm no-underline hover:bg-paper">{t("allProjects")}</Link>
        </Menu>
        {tabs.length > 0 && (
          <nav
            aria-label={project ? tp("nav") : t("label")}
            className="order-last -mx-4 flex h-11 w-[calc(100%+2rem)] items-stretch overflow-x-auto px-2 md:order-none md:mx-0 md:h-13 md:w-auto md:px-0"
          >
            {tabs.map(([href, label]) => (
              <Link
                key={href}
                href={href}
                aria-current={isActive(href) ? "page" : undefined}
                className={cx(
                  "flex shrink-0 items-center border-b-2 px-3 text-[13px] no-underline",
                  isActive(href)
                    ? "border-bar-accent font-semibold text-white hover:text-white"
                    : "border-transparent font-medium text-bar-muted hover:text-white",
                )}
              >
                {label}
              </Link>
            ))}
          </nav>
        )}
        <div className="ml-auto flex h-13 items-center gap-2">
          <Form action="/search" role="search">
            <label className="flex h-8 w-40 items-center gap-2 rounded border border-bar-line bg-bar-raised px-2.5 text-bar-muted focus-within:border-bar-accent sm:w-56 lg:w-80">
              <Icon name="search" />
              <input
                type="search"
                name="q"
                required
                aria-label={t("search")}
                placeholder={t("searchPlaceholder")}
                className="min-w-0 flex-1 bg-transparent text-[13px] text-white outline-none placeholder:text-bar-muted"
              />
            </label>
          </Form>
          {project && project.role !== "viewer" && (
            <Link href={`/p/${project.key}/tickets/new`} aria-label={t("newTicket")} className={button.primary}>
              <Icon name="plus" />
              <span className="hidden sm:inline">{t("newTicket")}</span>
            </Link>
          )}
          <div role="group" aria-label={t("language")} className="flex h-8 overflow-hidden rounded border border-bar-line text-xs font-semibold">
            {(["id", "en"] as const).map((l) => (
              <button
                key={l}
                type="button"
                aria-pressed={me.locale === l}
                onClick={() => setLocale(l)}
                className={cx("cursor-pointer px-2 uppercase", me.locale === l ? "bg-bar-raised text-white" : "text-bar-muted hover:text-white")}
              >
                {l}
              </button>
            ))}
          </div>
          <Menu align="right" label={t("account", { name: me.name })} summary={<Avatar name={me.name} className="size-8 bg-bar-line text-white" />}>
            <div className="border-b border-line-soft px-3 py-2">
              <div className="text-sm font-semibold">{me.name}</div>
              <div className="text-xs text-muted">{me.email}</div>
            </div>
            <Link href="/settings/profile" className="block px-3 py-2 text-sm text-ink no-underline hover:bg-paper hover:text-ink">{t("profile")}</Link>
            <SignOutButton label={t("signOut")} />
          </Menu>
        </div>
      </div>
    </header>
  );
}
