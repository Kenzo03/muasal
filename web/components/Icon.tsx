// Inline stroke icons in currentColor on a 16×16 grid. Callers pass a name;
// the paths live here so icons stay the same app-wide.
const paths = {
  logo: <path d="M8 2v5M8 7c-2.8 0-4.5 2-4.5 4.5M8 7c2.8 0 4.5 2 4.5 4.5M3.5 11.5V14M12.5 11.5V14M8 7v7" />,
  chevron: <path d="M4.5 6.5 8 10l3.5-3.5" />,
  chevronRight: <path d="M6.5 4.5 10 8l-3.5 3.5" />,
  plus: <path d="M8 3v10M3 8h10" />,
  search: (
    <>
      <circle cx="7" cy="7" r="4.5" />
      <path d="m10.5 10.5 3 3" />
    </>
  ),
  bug: (
    <>
      <circle cx="8" cy="9" r="3.5" />
      <path d="M8 5.5v-2M4.5 9h-2M13.5 9h-2M5.2 6.3 3.8 4.9M10.8 6.3l1.4-1.4M5.2 11.7l-1.4 1.4M10.8 11.7l1.4 1.4" />
    </>
  ),
  change: <path d="M2.5 5.5h10L10 3M13.5 10.5h-10L6 13" />,
  feature: <path d="m8 2.5 1.6 3.4 3.7.5-2.7 2.6.7 3.7L8 11l-3.3 1.7.7-3.7-2.7-2.6 3.7-.5z" />,
  lock: (
    <>
      <rect x="3.5" y="7" width="9" height="6.5" rx="1" />
      <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
    </>
  ),
  file: (
    <>
      <path d="M4 2.5h5l3 3v8H4z" />
      <path d="M9 2.5v3h3" />
    </>
  ),
  folder: <path d="M2.5 4.5h4l1.5 1.5h5.5v6.5h-11z" />,
  screen: (
    <>
      <rect x="2.5" y="3" width="11" height="10" rx="1.5" />
      <path d="M2.5 6h11" />
    </>
  ),
  edit: <path d="M10.5 3 13 5.5 6 12.5H3.5V10z" />,
  x: <path d="m4 4 8 8M12 4l-8 8" />,
  signOut: <path d="M9.5 3.5h3v9h-3M6.5 5.5 4 8l2.5 2.5M4 8h6" />,
  warning: (
    <>
      <path d="M8 2.5 14 13H2z" />
      <path d="M8 6.5v3M8 11.3v.01" />
    </>
  ),
  upload: <path d="M8 10.5V3M5 6l3-3 3 3M3 12.5h10" />,
  arrowRight: <path d="M3.5 8h9M9 4.5 12.5 8 9 11.5" />,
} satisfies Record<string, React.ReactNode>;

export type IconName = keyof typeof paths;

export default function Icon({ name, className = "size-4" }: { name: IconName; className?: string }) {
  return (
    <svg
      viewBox="0 0 16 16"
      aria-hidden="true"
      className={`shrink-0 fill-none stroke-current ${className}`}
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {paths[name]}
    </svg>
  );
}
