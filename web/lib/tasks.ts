// A ticket description's task list (MSL-55): "- [ ] step" lines tick on the
// ticket page and count as progress on the list and board.

const taskRe = /^(\s*(?:[-*+]|\d+[.)])\s+)\[[ xX]\]/;

/** The text with the task on this 1-based line ticked or not; unchanged when that line holds none. */
export function toggleTask(md: string, line: number, checked: boolean): string {
  const lines = md.split("\n");
  const i = line - 1;
  if (i < 0 || i >= lines.length || !taskRe.test(lines[i])) return md;
  lines[i] = lines[i].replace(taskRe, `$1[${checked ? "x" : " "}]`);
  return lines.join("\n");
}
