// Retakes the README and docs screenshots (docs/images/*.png) on a fresh
// `make up` stack: it creates the demo admin, loads the HRIS demo with
// `app eval --seed`, and shoots each screen at 1440×900 in the English UI.
//   make up && make screenshots
import { execFileSync } from "node:child_process";
import { chromium } from "@playwright/test";

const base = process.env.E2E_BASE_URL ?? "http://localhost";
const out = "../docs/images";
const compose = ["compose", "-f", "../deploy/compose.yaml", "--env-file", "../deploy/.env"];
const app = (...args) => execFileSync("docker", [...compose, "exec", "-T", "app", "/app", ...args], { encoding: "utf8" });

const email = "demo@zettra.dev";
const password = "demo-screenshots-passphrase-1";
const link = app("admin", "create-admin", "--email", email, "--name", "Dewi Lestari").match(/https?:\/\/\S+\/setup\/\S+/)?.[0];
if (!link) throw new Error("no setup link");
try {
  app("eval", "--seed"); // the golden set that follows may miss its targets with AI off; the demo is loaded either way
} catch (e) {
  if (!String(e.stdout).includes("Loading and indexing")) throw e;
}

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, baseURL: base });

// New users start in Indonesian; sign in there, then switch to English.
await page.goto(link);
await page.getByLabel("Kata sandi baru").fill(password);
await page.getByLabel("Ulangi kata sandi").fill(password);
await page.getByRole("button", { name: "Simpan kata sandi" }).click();
await page.getByRole("status").waitFor();
await page.goto("/login");
await page.getByLabel("Email").fill(email);
await page.getByLabel("Kata sandi").fill(password);
await page.getByRole("button", { name: "Masuk" }).click();
await page.waitForURL(`${base}/`);
const headers = { Origin: new URL(base).origin };
await page.request.patch("/api/v1/me", { data: { locale: "en" }, headers });
await page.context().addCookies([{ name: "locale", value: "en", url: base }]); // the web app reads the language from this cookie, set at sign-in

async function shoot(name, path, ready) {
  await page.goto(path);
  await ready();
  await page.mouse.move(0, 899); // no hover state in the shot
  await page.screenshot({ path: `${out}/${name}.png` });
  console.log(`${out}/${name}.png`);
}

const nodes = await (await page.request.get("/api/v1/projects/DEMO/nodes", { headers })).json();
const overtime = nodes.items.find((n) => n.name === "Overtime Approval");

await shoot("ticket", "/t/DEMO-2", () => page.getByRole("heading", { name: "Two-level overtime approval for Bumi Logistik" }).waitFor());
await shoot("menu-history", `/p/DEMO/modules/${overtime.id}`, () => page.getByRole("heading", { name: "Overtime Approval" }).first().waitFor());
await shoot("board", "/p/DEMO/board", () => page.getByRole("region", { name: "To do" }).waitFor());
await shoot("module-tree", "/p/DEMO/modules", () => page.getByText("Overtime Approval").first().waitFor());
await shoot("ask", "/ask", async () => {
  await page.getByPlaceholder("Ask about tickets, menus or decisions…").fill("Why does overtime approval skip the supervisor for Arunika?");
  await page.getByRole("button", { name: "Ask", exact: true }).last().click();
  await page.getByText("Keyword results").first().waitFor({ timeout: 30_000 });
});
await shoot("admin-clients", "/admin/clients", async () => {
  await page.getByText("Arunika Retail").first().click();
  await page.getByRole("heading", { name: "Edit Arunika Retail" }).waitFor();
});
await browser.close();
