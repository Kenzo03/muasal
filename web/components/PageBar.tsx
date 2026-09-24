// The white bar under the top bar: the page's title or path, then its tools.
export default function PageBar({ children }: { children: React.ReactNode }) {
  return <div className="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2 border-b border-line bg-white px-4 py-2 md:px-5">{children}</div>;
}
