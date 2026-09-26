// Fails when a message key or a {placeholder} exists in one language and not
// the other (FSD §18: CI fails on missing translation keys). Indonesian is the
// default language; English must match it key for key.
import { readFileSync, readdirSync } from "node:fs";

const dir = new URL("../messages/", import.meta.url);
const flatten = (obj, prefix = "") =>
  Object.entries(obj).flatMap(([k, v]) => (v && typeof v === "object" ? flatten(v, `${prefix}${k}.`) : [[`${prefix}${k}`, String(v)]]));
// ICU arguments open at an even brace depth: {count, plural, =0 {text}}
// holds one argument, count; the branches' text is not an argument.
const args = (text) => {
  const names = new Set();
  let depth = 0;
  for (let i = 0; i < text.length; i++) {
    if (text[i] === "{") {
      if (depth % 2 === 0) names.add(/^\s*(\w+)/.exec(text.slice(i + 1))?.[1]);
      depth++;
    } else if (text[i] === "}") depth--;
  }
  return [...names].sort().join(",");
};

const locales = readdirSync(dir).filter((f) => f.endsWith(".json")).map((f) => f.slice(0, -5));
const messages = Object.fromEntries(locales.map((l) => [l, new Map(flatten(JSON.parse(readFileSync(new URL(`${l}.json`, dir), "utf8"))))]));
const problems = [];
for (const [a, b] of locales.flatMap((a) => locales.filter((b) => b !== a).map((b) => [a, b]))) {
  for (const [key, text] of messages[a]) {
    if (!messages[b].has(key)) problems.push(`${b}.json is missing ${key}`);
    else if (a < b && args(text) !== args(messages[b].get(key))) problems.push(`${key}: {${args(text)}} in ${a}, {${args(messages[b].get(key))}} in ${b}`);
  }
}
if (problems.length > 0) {
  console.error(problems.join("\n"));
  process.exit(1);
}
console.log(`i18n: ${locales.join(", ")} match (${messages[locales[0]].size} keys)`);
