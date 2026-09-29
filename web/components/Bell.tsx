"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import Icon from "./Icon";
import Menu from "./Menu";
import { api } from "@/lib/api";
import type { components } from "@/lib/api-types";
import { utc } from "@/lib/format";
import { button, cx } from "@/lib/ui";

type Notification = components["schemas"]["Notification"];

// The notification bell (FSD §8.10): an unread count, the latest 50, live
// updates over SSE while a tab is open, and an OS notification when the tab is
// hidden and the user opted in.
export default function Bell({ browser }: { browser: boolean }) {
  const t = useTranslations("bell");
  const locale = useLocale();
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
        const os = new Notification(t("title"), { body: text(n), tag: `muasal-${n.id}` });
        os.onclick = () => {
          window.focus();
          open(n);
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
      default:
        return t("jobDone", { name: String(n.payload.name ?? "") });
    }
  }

  async function open(n: Notification) {
    if (!n.read) {
      await api.POST("/notifications/read", { body: { id: n.id } });
      setItems((xs) => xs.map((x) => (x.id === n.id ? { ...x, read: true } : x)));
      setUnread((u) => Math.max(0, u - 1));
    }
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
          {items.map((n) => (
            <li key={n.id}>
              <button
                type="button"
                onClick={() => open(n)}
                className={cx("flex w-full cursor-pointer flex-col items-start gap-0.5 px-3 py-2 text-left text-[13px] hover:bg-paper", !n.read && "bg-accent-soft/40")}
              >
                <span className={cx(!n.read && "font-semibold")}>{text(n)}</span>
                {n.type === "comment" || n.type === "mention" ? <span className="line-clamp-2 text-xs text-muted">{String(n.payload.excerpt ?? "")}</span> : null}
                <span className="text-xs text-muted">{utc(n.created_at, locale)}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </Menu>
  );
}
