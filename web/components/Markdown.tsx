import ReactMarkdown, { type Components } from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";
import { remarkMentions, type Person } from "@/lib/mentions";
import { cx } from "@/lib/ui";

// Descriptions and comments are markdown (FSD §8.1, §8.7). Raw HTML is never
// rendered, and rehype-sanitize strips anything unsafe from the rest. Images
// show only when they are this app's attachments, which Go serves after a
// visibility check; any other image stays a plain link, so a ticket cannot
// load outside content (§18.1: nothing leaves the server).
const attachment = /^\/api\/v1\/attachments\/\d+$/;

const components: Components = {
  h1: ({ children }) => <h3 className="mt-3 text-base font-semibold first:mt-0">{children}</h3>,
  h2: ({ children }) => <h3 className="mt-3 text-[15px] font-semibold first:mt-0">{children}</h3>,
  h3: ({ children }) => <h4 className="mt-2 text-sm font-semibold first:mt-0">{children}</h4>,
  p: ({ children }) => <p className="my-1.5 first:mt-0 last:mb-0">{children}</p>,
  ul: ({ children }) => <ul className="my-1.5 list-disc pl-5">{children}</ul>,
  ol: ({ children }) => <ol className="my-1.5 list-decimal pl-5">{children}</ol>,
  blockquote: ({ children }) => <blockquote className="my-1.5 border-l-2 border-line pl-3 text-muted">{children}</blockquote>,
  code: ({ children, className }) => <code className={cx("rounded bg-paper px-1 font-mono text-[12.5px]", className)}>{children}</code>,
  pre: ({ children }) => <pre className="my-2 overflow-x-auto rounded border border-line-soft bg-paper p-2.5 [&_code]:bg-transparent [&_code]:p-0">{children}</pre>,
  table: ({ children }) => (
    <div className="my-2 overflow-x-auto">
      <table className="border-collapse text-[13px] [&_td]:border [&_td]:border-line [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-line [&_th]:bg-paper [&_th]:px-2 [&_th]:py-1 [&_th]:text-left">{children}</table>
    </div>
  ),
  a: ({ href, children }) => {
    if (href?.startsWith("#mention-")) {
      return <span title={`@${href.slice(9)}`} className="rounded bg-accent-soft px-1 font-semibold text-accent-strong">{children}</span>;
    }
    const external = href !== undefined && /^https?:\/\//.test(href);
    return (
      <a href={href} {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}>
        {children}
      </a>
    );
  },
  img: ({ src, alt }) =>
    typeof src === "string" && attachment.test(src) ? (
      <a href={src} target="_blank" rel="noopener" className="my-1.5 inline-block">
        <img src={src} alt={alt ?? ""} loading="lazy" className="max-h-96 max-w-full rounded border border-line" />
      </a>
    ) : (
      <a href={typeof src === "string" ? src : undefined} target="_blank" rel="noopener noreferrer">{alt || src?.toString()}</a>
    ),
};

// people turns their @handles into name chips (MSL-30).
export default function Markdown({ text, className, people }: { text: string; className?: string; people?: Person[] }) {
  return (
    <div className={cx("break-words text-sm leading-relaxed", className)}>
      <ReactMarkdown remarkPlugins={people?.length ? [remarkGfm, remarkMentions(people)] : [remarkGfm]} rehypePlugins={[rehypeSanitize]} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  );
}
