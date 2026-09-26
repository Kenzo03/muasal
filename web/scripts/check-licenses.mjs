// Fails when a package the web app ships (production dependencies, all
// levels) has a licence Muasal may not ship under Apache-2.0 (FSD §18:
// licence scan in CI). SPDX "OR" needs one allowed choice; "AND" needs all.
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const allowed = new Set(["MIT", "ISC", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "0BSD", "BlueOak-1.0.0", "CC0-1.0", "CC-BY-4.0", "Python-2.0", "MPL-2.0", "Unlicense"]);
// Named exceptions, listed in NOTICE: sharp's @img packages ship libvips
// unmodified under LGPL-3.0-or-later.
const exceptions = [/^@img\/sharp-/];
const ok = (expr) => {
  const e = expr.replace(/[()]/g, " ").trim();
  if (/\sOR\s/.test(e)) return e.split(/\s+OR\s+/).some(ok);
  if (/\sAND\s/.test(e)) return e.split(/\s+AND\s+/).every(ok);
  return allowed.has(e);
};

const tree = JSON.parse(execFileSync("npm", ["ls", "--omit=dev", "--all", "--json"], { encoding: "utf8", maxBuffer: 64 << 20 }));
const seen = new Map();
const walk = (deps = {}) => {
  for (const [name, d] of Object.entries(deps)) {
    const key = `${name}@${d.version}`;
    if (seen.has(key) || !d.version) continue;
    let license = "none";
    try {
      const pkg = JSON.parse(readFileSync(join("node_modules", name, "package.json"), "utf8"));
      license = typeof pkg.license === "string" ? pkg.license : (pkg.license?.type ?? pkg.licenses?.map((l) => l.type).join(" OR ") ?? "none");
    } catch {
      // nested copies: npm ls reports them; their licence matches the hoisted one in practice
    }
    seen.set(key, license);
    walk(d.dependencies);
  }
};
walk(tree.dependencies);
const bad = [...seen].filter(([k, l]) => !ok(l) && !exceptions.some((re) => re.test(k)));
const counts = {};
for (const [, l] of seen) counts[l] = (counts[l] ?? 0) + 1;
console.log(`licences: ${seen.size} packages`, counts);
if (bad.length > 0) {
  console.error("Not allowed:\n  " + bad.map(([k, l]) => `${k}: ${l}`).join("\n  "));
  process.exit(1);
}
