// The page's title or path, then its tools, under the top bar. Every page puts
// its h1 here, so the title size lives here too.
export default function PageBar({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2 px-4 pb-1 md:px-5 [&_h1]:text-[22px] [&_h1]:font-extrabold [&_h1]:tracking-[-0.02em]">
      {children}
    </div>
  );
}
