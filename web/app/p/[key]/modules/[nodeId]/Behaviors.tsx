import Link from "next/link";
import { getLocale, getTimeZone, getTranslations } from "next-intl/server";
import { dayOf } from "@/lib/format";
import type { Behavior } from "@/lib/problem";
import { chip, cx, panel, sectionTitle } from "@/lib/ui";

// The Behaviors tab (FSD §7.4, story 5): the decisions in force, "All clients"
// first, then each client in the user's scope, so QA checks what a client
// should get before filing a bug. The API returns them in that order.
export default async function Behaviors({ items }: { items: Behavior[] }) {
  const t = await getTranslations("nodePage");
  const locale = await getLocale();
  const timeZone = await getTimeZone();
  if (items.length === 0) return <p className="text-muted">{t("noBehaviors")}</p>;
  const groups: { name: string; items: Behavior[] }[] = [];
  for (const b of items) {
    const name = b.client?.name ?? t("allClients");
    const last = groups.at(-1);
    if (last?.name === name) last.items.push(b);
    else groups.push({ name, items: [b] });
  }
  return (
    <div className="flex max-w-4xl flex-col gap-5">
      {groups.map((g) => (
        <section key={g.name} aria-label={g.name} className="flex flex-col gap-2">
          <h2 className={sectionTitle}>{g.name}</h2>
          <ul className={cx(panel, "divide-y divide-line-soft")}>
            {g.items.map((b) => (
              <li key={b.key} className="flex flex-col gap-1.5 px-5 py-4">
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
                  <Link href={`/t/${b.key}`} className="text-[13px] font-bold no-underline">{b.key}</Link>
                  <span className="text-sm font-bold text-ink">{b.title}</span>
                  {/* MSL-13: imported history has no confirmed record yet; it counts, marked. */}
                  {b.unconfirmed && <span className={cx(chip, "bg-warn-soft text-warn")} title={t("unconfirmedHint")}>{t("unconfirmed")}</span>}
                  {b.closed_at && <span className="ml-auto">{dayOf(b.closed_at, locale, timeZone)}</span>}
                </div>
                {!b.unconfirmed && <p className="whitespace-pre-wrap text-sm leading-relaxed">{b.what_changed}</p>}
                {b.why && <p className="whitespace-pre-wrap text-[13px] text-muted">{t(b.unconfirmed ? "reasonOnly" : "because", { why: b.why })}</p>}
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
