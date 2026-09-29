// Inline stroke icons in currentColor on a 16×16 grid. Callers pass a name;
// the paths live here so icons stay the same app-wide.
const paths = {
  logo: <path d="M8 2v5M8 7c-2.8 0-4.5 2-4.5 4.5M8 7c2.8 0 4.5 2 4.5 4.5M3.5 11.5V14M12.5 11.5V14M8 7v7" />,
  chevron: <path d="M4.5 6.5 8 10l3.5-3.5" />,
  chevronRight: <path d="M6.5 4.5 10 8l-3.5 3.5" />,
  plus: <path d="M8 3v10M3 8h10" />,
  bell: <path d="M4 11.5V7a4 4 0 0 1 8 0v4.5l1 1H3l1-1ZM6.5 13.5a1.5 1.5 0 0 0 3 0" />,
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
  check: (
    <>
      <circle cx="8" cy="8" r="5.5" />
      <path d="m5.5 8 1.8 1.8 3.2-3.6" />
    </>
  ),
  grip: (
    <>
      <circle cx="6" cy="4" r=".9" />
      <circle cx="10" cy="4" r=".9" />
      <circle cx="6" cy="8" r=".9" />
      <circle cx="10" cy="8" r=".9" />
      <circle cx="6" cy="12" r=".9" />
      <circle cx="10" cy="12" r=".9" />
    </>
  ),
  upload: <path d="M8 10.5V3M5 6l3-3 3 3M3 12.5h10" />,
  download: <path d="M8 3v7.5M5 7.5l3 3 3-3M3 12.5h10" />,
  arrowRight: <path d="M3.5 8h9M9 4.5 12.5 8 9 11.5" />,
  // The sidebar's pages.
  home: (
    <>
      <path d="M2.5 7.2 8 2.8l5.5 4.4" />
      <path d="M4 6.2V13h3V9.8h2V13h3V6.2" />
    </>
  ),
  sparkle: (
    <>
      <path d="M7 2.5c.4 2.5 1.7 3.8 4.2 4.2-2.5.4-3.8 1.7-4.2 4.2-.4-2.5-1.7-3.8-4.2-4.2 2.5-.4 3.8-1.7 4.2-4.2Z" />
      <path d="M12.3 10.3v3.2M10.7 11.9h3.2" />
    </>
  ),
  board: (
    <>
      <rect x="2.5" y="2.5" width="3" height="11" rx="1" />
      <rect x="6.5" y="2.5" width="3" height="7" rx="1" />
      <rect x="10.5" y="2.5" width="3" height="9" rx="1" />
    </>
  ),
  list: <path d="M6 4h7.5M6 8h7.5M6 12h7.5M2.8 4h.01M2.8 8h.01M2.8 12h.01" />,
  tree: (
    <>
      <rect x="2.5" y="2" width="4.5" height="3" rx=".8" />
      <rect x="9" y="6.5" width="4.5" height="3" rx=".8" />
      <rect x="9" y="11" width="4.5" height="3" rx=".8" />
      <path d="M4.7 5v7.5H9M4.7 8H9" />
    </>
  ),
  notes: (
    <>
      <rect x="3.5" y="2.5" width="9" height="11" rx="1.2" />
      <path d="M6 5.5h4M6 8h4M6 10.5h2.5" />
    </>
  ),
  summary: <path d="M3 3.5h10M3 6.5h10M3 9.5h6.5M3 12.5h4" />,
  sliders: (
    <>
      <path d="M2.5 4.5h7M12.5 4.5h1M2.5 11.5h1M6.5 11.5h7" />
      <circle cx="11" cy="4.5" r="1.5" />
      <circle cx="5" cy="11.5" r="1.5" />
    </>
  ),
  shield: <path d="M8 2.2 13 4.2v3.6c0 3-2.1 5-5 6-2.9-1-5-3-5-6V4.2Z" />,
  users: (
    <>
      <circle cx="6" cy="5.5" r="2.3" />
      <path d="M2 13c.4-2.2 1.9-3.5 4-3.5s3.6 1.3 4 3.5M10.5 3.4a2.3 2.3 0 0 1 0 4.4M12 9.7c1.1.5 1.8 1.7 2 3.3" />
    </>
  ),
  building: <path d="M3 13.5V3.5h6.5v10M9.5 6.5H13v7M5 5.5h2M5 8h2M5 10.5h2M2 13.5h12" />,
  archive: (
    <>
      <rect x="2.5" y="3" width="11" height="3" rx=".8" />
      <path d="M3.5 6v6.5h9V6M6.5 8.5h3" />
    </>
  ),
  pulse: <path d="M2 8.5h2.5L6 5l2.5 7L10 8.5h4" />,
  menu: <path d="M2.5 4.5h11M2.5 8h11M2.5 11.5h11" />,
  message: <path d="M3 3.5h10v7H7.2L4 13v-2.5H3z" />,
  thumbUp: <path d="M5 7.5v6H3v-6zM5 7.5 7.6 3a1.4 1.4 0 0 1 1.6 1.7l-.5 2.3h3.6a1.3 1.3 0 0 1 1.3 1.5l-.8 4.3a1.4 1.4 0 0 1-1.4 1.2H5" />,
  calendar: (
    <>
      <rect x="2.5" y="3.5" width="11" height="10" rx="1.5" />
      <path d="M2.5 6.5h11M5.5 2v3M10.5 2v3" />
    </>
  ),
  help: (
    <>
      <circle cx="8" cy="8" r="5.5" />
      <path d="M6.4 6.3a1.7 1.7 0 0 1 3.2.7c0 1.2-1.6 1.4-1.6 2.5M8 11.2v.01" />
    </>
  ),
  xCircle: (
    <>
      <circle cx="8" cy="8" r="5.5" />
      <path d="m6 6 4 4M10 6l-4 4" />
    </>
  ),
  updown: <path d="M5.5 6 8 3.5 10.5 6M5.5 10 8 12.5 10.5 10" />,
  collapse: <path d="M8 4.5 4.5 8 8 11.5M12 4.5 8.5 8l3.5 3.5" />,
  expand: <path d="M4 4.5 7.5 8 4 11.5M8 4.5 11.5 8 8 11.5" />,
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
