"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTimeZone, useTranslations } from "next-intl";
import Icon from "./Icon";
import Menu from "./Menu";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { dateTime } from "@/lib/format";
import { bursts } from "@/lib/notifications";
import { button, cx } from "@/lib/ui";

type Notification = components["schemas"]["Notification"];

// The notification bell (FSD §8.10): an unread count, the latest 50, live
// updates over SSE while a tab is open, and an OS notification when the tab is
// hidden and the user opted in.
export default function Bell({ browser }: { browser: boolean }) {
  const t = useTranslations("bell");
  const locale = useLocale();
  const timeZone = useTimeZone();
  const router = useRouter();
  const [items, setItems] = useState<Notification[]>([]);
  const [unread, setUnread] = useState(0);

  useEffect(() => {
    let live = true;
    api.GET("/notifications").then(({ data }) => {
      if (live && data) {
        setItems(data.items);
        setUnread(data.unread);
      }
    });
    const es = new EventSource("/api/v1/notifications/stream");
    es.addEventListener("notification", (e) => {
      const n = JSON.parse((e as MessageEvent).data) as Notification;
      setItems((xs) => [n, ...xs.filter((x) => x.id !== n.id)].slice(0, 50));
      setUnread((u) => u + 1);
      if (browser && document.hidden && "Notification" in window && Notification.permission === "granted") {
        const os = new Notification(t("title"), { body: text(n), tag: `zettra-${n.id}` });
        os.onclick = () => {
          window.focus();
          open([n]);
        };
      }
    });
    return () => {
      live = false;
      es.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [browser]);

  function text(n: Notification): string {
    const who = n.actor?.name ?? t("someone");
    const ticket = n.ticket_key ? `${n.ticket_key} ${n.ticket_title ?? ""}`.trim() : "";
    switch (n.type) {
      case "assigned":
        return t("assigned", { who, ticket });
      case "comment":
        return t("comment", { who, ticket });
      case "mention":
        return t("mention", { who, ticket });
      case "status":
        return t("status", { who, ticket, status: String(n.payload.status ?? "") });
      case "due": // MSL-52: the morning reminder
        return t(`due.${String(n.payload.when) as "today" | "tomorrow" | "overdue"}`, { ticket });
      default:
        return t("jobDone", { name: String(n.payload.name ?? "") });
    }
  }

  // What the earlier notifications of a burst add, without repeating the ticket (MSL-28).
  function also(n: Notification): string {
    return n.type === "status" ? t("did.status", { status: String(n.payload.status ?? "") }) : t(`did.${n.type as "assigned" | "comment" | "mention"}`);
  }

  // Opening a burst reads all of it.
  async function open(g: Notification[]) {
    const ids = g.filter((n) => !n.read).map((n) => n.id);
    await Promise.all(ids.map((id) => api.POST("/notifications/read", { body: { id } })));
    if (ids.length > 0) {
      setItems((xs) => xs.map((x) => (ids.includes(x.id) ? { ...x, read: true } : x)));
      setUnread((u) => Math.max(0, u - ids.length));
    }
    const n = g[0];
    router.push(n.ticket_key ? `/t/${n.ticket_key}` : String(n.payload.link ?? "/"));
  }

  async function readAll() {
    await api.POST("/notifications/read", { body: {} });
    setItems((xs) => xs.map((x) => ({ ...x, read: true })));
    setUnread(0);
  }

  return (
    <Menu
      align="right"
      label={unread > 0 ? t("labelUnread", { count: unread }) : t("label")}
      summaryClassName="relative flex size-10 items-center justify-center rounded-[11px] border border-line bg-white text-ink-soft hover:text-ink"
      summary={
        <>
          <Icon name="bell" />
          {unread > 0 && (
            <span data-testid="bell-count" className="absolute -right-1.5 -top-1.5 min-w-4 rounded-full bg-accent px-1 text-center text-[10px] font-bold leading-4 text-white">
              {unread > 99 ? "99+" : unread}
            </span>
          )}
        </>
      }
    >
      <div className="flex items-center gap-2 border-b border-line-soft px-3 py-2">
        <span className="text-sm font-semibold">{t("title")}</span>
        {unread > 0 && (
          <button type="button" onClick={readAll} className={cx(button.quiet, "ml-auto text-xs")}>{t("readAll")}</button>
        )}
      </div>
      {items.length === 0 ? (
        <p className="px-3 py-3 text-[13px] text-muted">{t("none")}</p>
      ) : (
        <ul className="max-h-96 w-80 overflow-y-auto">
          {bursts(items).map((g) => {
            const [n, ...earlier] = g;
            const unreadHere = g.some((x) => !x.read);
            const said = g.find((x) => x.type === "comment" || x.type === "mention");
            return (
              <li key={n.id}>
                <button
                  type="button"
                  onClick={() => open(g)}
                  className={cx("flex w-full cursor-pointer flex-col items-start gap-0.5 px-3 py-2 text-left text-[13px] hover:bg-paper", unreadHere && "bg-accent-soft/40")}
                >
                  <span className={cx(unreadHere && "font-semibold")}>{text(n)}</span>
                  {earlier.length > 0 && <span className="text-xs text-ink-soft">{t("also", { actions: [...new Set(earlier.map(also))].join(", ") })}</span>}
                  {said && <span className="line-clamp-2 text-xs text-muted">{String(said.payload.excerpt ?? "")}</span>}
                  <span className="text-xs text-muted">{dateTime(n.created_at, locale, timeZone)}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </Menu>
  );
}
