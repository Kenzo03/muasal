import { expect, test, type Page } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

function api(page: Page) {
  return async (method: string, path: string, data?: unknown) => {
    const origin = new URL(page.url()).origin;
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.status() === 204 ? null : res.json();
  };
}

// FSD §8.10 and AC-TK-9: with the app open, Rina's bell counts an assignment
// within 5 seconds; an @mention picked from the suggestions notifies her too.
test("a member sees assignments and mentions arrive in the bell", async ({ page, browser }) => {
  test.setTimeout(120_000);
  const run = Date.now().toString(36).toUpperCase();
  const key = `N${run.slice(-6)}`;
  const rinaEmail = `rina-${run.toLowerCase()}@example.com`;
  const handle = `rina-${run.toLowerCase()}`;

  await setPassword(page, process.env.E2E_BELL_ADMIN_LINK!, "e2e-bell-admin-passphrase-3");
  await signIn(page, process.env.E2E_BELL_ADMIN_EMAIL!, "e2e-bell-admin-passphrase-3");
  const asAdmin = api(page);
  await asAdmin("POST", "/projects", { key, name: `Bell ${run}` });
  const menu = await asAdmin("POST", `/projects/${key}/nodes`, { type: "menu", name: "Overtime Approval" });
  const rina = await asAdmin("POST", "/admin/users", { name: "Rina PM", email: rinaEmail });
  const me = await asAdmin("GET", "/me");
  await asAdmin("PUT", `/projects/${key}/members`, {
    members: [
      { email: rinaEmail, role: "member", all_clients: true },
      { email: me.email, role: "admin", all_clients: true },
    ],
  });

  const pm = await (await browser.newContext()).newPage();
  await setPassword(pm, rina.setup_link.url, "e2e-rina-bell-passphrase-4");
  await signIn(pm, rinaEmail, "e2e-rina-bell-passphrase-4");
  await expect(pm.getByLabel("Notifikasi", { exact: true })).toBeVisible();

  // AC-TK-9: the admin assigns a ticket; Rina's open tab counts it within 5 seconds.
  const ticket = await asAdmin("POST", `/projects/${key}/tickets`, {
    type: "bug", title: `Overtime approval stuck ${run}`, node_ids: [menu.id], assignee_id: rina.user.id,
  });
  await expect(pm.getByTestId("bell-count")).toHaveText("1", { timeout: 5_000 });
  await pm.getByLabel("Notifikasi, 1 belum dibaca").click();
  await pm.getByRole("button", { name: new RegExp(`menugaskan ${ticket.key}`) }).click();
  await expect(pm).toHaveURL(new RegExp(`/t/${ticket.key}$`));
  await expect(pm.getByTestId("bell-count")).toHaveCount(0);

  // The admin mentions Rina by picking her from the suggestions.
  await page.goto(`/t/${ticket.key}`);
  const box = page.getByLabel("Tulis komentar");
  await box.pressSequentially(`Bisa dicek, @${handle.slice(0, 6)}`);
  await page.getByRole("listbox", { name: "Anggota yang dapat disebut" }).getByRole("button", { name: /Rina PM/ }).click();
  await expect(box).toHaveValue(`Bisa dicek, @${handle} `);
  await page.getByRole("button", { name: "Kirim" }).click();
  await expect(pm.getByTestId("bell-count")).toHaveText("1", { timeout: 5_000 });
  await pm.getByLabel("Notifikasi, 1 belum dibaca").click();
  await expect(pm.getByRole("button", { name: new RegExp(`menyebut Anda di ${ticket.key}`) })).toBeVisible();
});
