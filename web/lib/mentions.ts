// MSL-30: an @handle of someone the ticket can mention shows as their name.
// The handle is the part of their email before the @, as the server matches it.

export type Person = { handle: string; name: string };
type MdNode = { type: string; value?: string; url?: string; children?: MdNode[] };

const mentionRe = /(^|[^\w@.])@([a-zA-Z0-9][a-zA-Z0-9._-]{0,63})/g;

/** A remark plugin: plain text only, so code and links keep their @s. */
export function remarkMentions(people: Person[]) {
  const byHandle = new Map(people.map((p) => [p.handle.toLowerCase(), p.name]));
  const split = (value: string): MdNode[] => {
    const out: MdNode[] = [];
    let last = 0;
    for (const m of value.matchAll(mentionRe)) {
      const handle = m[2].replace(/\.+$/, ""); // "@bayu." ends a sentence
      const name = byHandle.get(handle.toLowerCase());
      if (!name) continue;
      const at = m.index + m[1].length;
      if (at > last) out.push({ type: "text", value: value.slice(last, at) });
      out.push({ type: "link", url: `#mention-${handle}`, children: [{ type: "text", value: name }] });
      last = at + 1 + handle.length;
    }
    if (last < value.length) out.push({ type: "text", value: value.slice(last) });
    return out;
  };
  const walk = (node: MdNode) => {
    if (!node.children || node.type === "link") return;
    node.children = node.children.flatMap((c) => {
      if (c.type === "text") return split(c.value ?? "");
      walk(c);
      return [c];
    });
  };
  return () => (tree: MdNode) => walk(tree);
}
