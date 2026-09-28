import Icon from "./Icon";
import type { Priority, Ref, TicketType } from "@/lib/problem";
import { chip, cx } from "@/lib/ui";

// A client keeps one tint everywhere, picked from its id; core work is neutral.
const tints = [
  "bg-[#DDEEE9] text-[#145B4E]",
  "bg-[#F6E3CF] text-[#7A3F0A]",
  "bg-[#EAE3F4] text-[#4A3280]",
  "bg-[#DCE8F5] text-[#1F4F82]",
  "bg-[#F5DDE3] text-[#8A2442]",
  "bg-[#E6ECD6] text-[#4B5A1E]",
];

// A project with one client (or none) needs no client chips or filter: every
// ticket is that client's or core work. ponytail: the list is the user's own
// client scope, so a member scoped to one client of a larger project loses the
// chips too; a client count on the project API would tell the two apart.
export const showsClients = (clients: readonly unknown[]) => clients.length > 1;

export function ClientChip({ client, coreLabel }: { client?: Ref | null; coreLabel: string }) {
  return <span className={cx(chip, client ? tints[client.id % tints.length] : "bg-well text-[#4A423C]")}>{client?.name ?? coreLabel}</span>;
}

const priorityTone: Record<Priority, string> = {
  urgent: "bg-danger-soft text-danger",
  high: "bg-[#FBE8D0] text-[#874700]",
  medium: "bg-ground text-[#4A423C]",
  low: "bg-ground text-muted",
};

export function PriorityChip({ priority, label }: { priority: Priority; label: string }) {
  return <span className={cx(chip, priorityTone[priority])}>{label}</span>;
}

export const typeIcon = {
  bug: ["bug", "text-[#B23A1A]"],
  change_request: ["change", "text-[#2F6FAF]"],
  feature: ["feature", "text-[#6C5BB5]"],
} as const;

export function TypeIcon({ type, label, className = "size-3.5" }: { type: TicketType; label: string; className?: string }) {
  const [name, tone] = typeIcon[type];
  return (
    <span role="img" aria-label={label} title={label} className={cx("inline-flex", tone)}>
      <Icon name={name} className={className} />
    </span>
  );
}

export function StatusDot({ color, className = "size-2" }: { color: string; className?: string }) {
  return <span aria-hidden="true" className={cx("inline-block shrink-0 rounded-full", className)} style={{ background: color }} />;
}

/** The first letters of the first two words: "Rina Wijaya" → "RW". */
export function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0].toUpperCase())
    .join("");
}

export function Avatar({ name, className = "size-6 bg-well text-ink" }: { name: string; className?: string }) {
  return (
    <span title={name} className={cx("inline-flex shrink-0 items-center justify-center rounded-full text-[10px] font-semibold", className)}>
      {initials(name)}
    </span>
  );
}
