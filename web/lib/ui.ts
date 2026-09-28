// Class lists of the Terakota Lembut design's building blocks, shared by every
// screen so buttons, fields and panels look the same everywhere.

export const cx = (...parts: (string | false | null | undefined)[]) => parts.filter(Boolean).join(" ");

const buttonBase =
  "inline-flex h-9 shrink-0 cursor-pointer items-center justify-center gap-1.5 whitespace-nowrap rounded-[10px] px-3.5 text-[13.5px] font-semibold no-underline disabled:cursor-default disabled:opacity-40";

export const button = {
  primary: `${buttonBase} bg-accent text-white shadow-[0_1px_2px_rgba(120,40,20,0.25)] hover:bg-accent-strong hover:text-white`,
  secondary: `${buttonBase} border border-line bg-white text-ink hover:bg-paper hover:text-ink`,
  danger: `${buttonBase} border border-danger-line bg-white text-danger hover:bg-danger-soft hover:text-danger`,
  // Looks like a link, acts like a button.
  quiet: "inline-flex cursor-pointer items-center gap-1 text-[13px] font-semibold text-link hover:text-link-hover hover:underline disabled:opacity-40",
};

// A control marked aria-invalid gets a red border, so an error points at its field.
export const field = {
  input: "h-10 rounded-[10px] border border-field bg-white px-3 text-sm text-ink placeholder:text-muted disabled:opacity-50 aria-invalid:border-danger",
  compact: "h-9 rounded-[10px] border border-line bg-white px-2.5 text-[13px] text-ink aria-invalid:border-danger",
  textarea: "rounded-xl border border-field bg-white px-3 py-2.5 text-sm leading-relaxed text-ink placeholder:text-muted aria-invalid:border-danger",
  label: "flex flex-col gap-1.5 text-[13px] font-semibold text-ink",
  hint: "text-xs text-muted",
  error: "text-[13px] text-danger",
};

// Cards float on the ground with a faint line and a soft shadow.
const lift = "shadow-[0_1px_2px_rgba(43,36,32,0.04),0_4px_16px_rgba(43,36,32,0.04)]";

export const panel = `rounded-2xl border border-line bg-white ${lift}`;
export const chip = "inline-flex items-center gap-1 whitespace-nowrap rounded-md px-2 text-[11.5px] font-semibold leading-[22px]";
// A radio or checkbox drawn as a pill; its box stays visible, so the choice reads at a glance.
export const choice =
  "flex h-10 cursor-pointer items-center gap-2 rounded-full border border-line bg-white px-3.5 text-[13px] font-semibold text-ink-soft hover:border-field has-[:checked]:border-accent has-[:checked]:bg-accent-soft has-[:checked]:text-accent-strong [&>input]:size-3.5 [&>input]:accent-accent";
export const sectionTitle = "text-[13px] font-bold text-muted";

export const table = {
  wrap: `overflow-x-auto rounded-2xl border border-line bg-white ${lift}`,
  table: "w-full border-collapse text-left text-[13px]",
  head: "border-b border-line bg-paper text-xs text-muted",
  th: "px-3.5 py-2.5 font-semibold",
  row: "border-b border-line-soft last:border-0",
  td: "px-3.5 py-2.5 align-top",
};
