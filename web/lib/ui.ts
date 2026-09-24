// Class lists of the Terakota design's building blocks, shared by every screen
// so buttons, fields and panels look the same everywhere.

export const cx = (...parts: (string | false | null | undefined)[]) => parts.filter(Boolean).join(" ");

const buttonBase =
  "inline-flex h-8 shrink-0 cursor-pointer items-center justify-center gap-1.5 whitespace-nowrap rounded px-3 text-[13px] no-underline disabled:cursor-default disabled:opacity-40";

export const button = {
  primary: `${buttonBase} bg-accent font-semibold text-white hover:bg-accent-strong hover:text-white`,
  secondary: `${buttonBase} border border-line bg-white text-ink hover:bg-paper hover:text-ink`,
  danger: `${buttonBase} border border-danger-line bg-white text-danger hover:bg-danger-soft hover:text-danger`,
  // Looks like a link, acts like a button.
  quiet: "inline-flex cursor-pointer items-center gap-1 text-[13px] font-medium text-link hover:text-link-hover hover:underline disabled:opacity-40",
};

export const field = {
  input: "h-[34px] rounded border border-field bg-white px-2.5 text-sm text-ink placeholder:text-muted disabled:opacity-50",
  compact: "h-8 rounded border border-line bg-white px-2 text-[13px] text-ink",
  textarea: "rounded border border-field bg-white px-2.5 py-2 text-sm leading-relaxed text-ink placeholder:text-muted",
  label: "flex flex-col gap-1 text-[13px] font-medium text-ink",
  hint: "text-xs text-muted",
  error: "text-[13px] text-danger",
};

export const panel = "rounded border border-line bg-white";
export const chip = "inline-flex items-center gap-1 whitespace-nowrap rounded-[3px] px-1.5 text-[11px] font-semibold leading-5";
export const sectionTitle = "text-xs font-semibold uppercase tracking-[0.04em] text-muted";

export const table = {
  wrap: "overflow-x-auto rounded border border-line bg-white",
  table: "w-full border-collapse text-left text-[13px]",
  head: "border-b border-line bg-paper text-xs text-muted",
  th: "px-3 py-2 font-semibold",
  row: "border-b border-line-soft last:border-0",
  td: "px-3 py-2 align-top",
};
