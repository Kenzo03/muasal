import { expect, test } from "@playwright/test";
import { setPassword, signIn } from "./helpers";

// The board: a project admin can close the setup steps for good (in this
// browser), and a long status column scrolls on its own, not the page.
test("the setup steps close and a long column scrolls by itself", async ({ page }) => {
  const run = Date.now().toString(36).toUpperCase();
  const key = `B${run.slice(-6)}`;
  const password = "e2e-board-admin-passphrase-8";
  await setPassword(page, process.env.E2E_BOARD_ADMIN_LINK!, password);
  await signIn(page, process.env.E2E_BOARD_ADMIN_EMAIL!, password);
  const origin = new URL(page.url()).origin;
  const call = async (method: string, path: string, data?: unknown) => {
    const res = await page.request.fetch(`/api/v1${path}`, { method, data, headers: { Origin: origin } });
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBeTruthy();
    return res.json();
  };
  await call("POST", "/projects", { key, name: `Board ${run}` });
  const node = await call("POST", `/projects/${key}/nodes`, { type: "module", name: "Board" });
  for (let i = 1; i <= 15; i++) {
    await call("POST", `/projects/${key}/tickets`, { type: "bug", title: `Long column ${i}`, node_ids: [node.id] });
  }

  // The setup steps show for a new project until closed; closed stays closed.
  await page.goto(`/p/${key}/board`);
  const setup = page.getByRole("region", { name: "Siapkan proyek ini" });
  await expect(setup).toBeVisible();
  await page.getByRole("button", { name: "Sembunyikan langkah persiapan" }).click();
  await expect(setup).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("region", { name: "To do" })).toBeVisible();
  await expect(setup).toHaveCount(0);

  // Fifteen cards: the To do column's card list scrolls, the page does not.
  const todo = page.getByRole("region", { name: "To do" });
  await expect(todo.getByRole("article")).toHaveCount(15);
  const scrolls = await todo.evaluate((el) =>
    [...el.querySelectorAll("*")].some((c) => getComputedStyle(c).overflowY === "auto" && c.scrollHeight > c.clientHeight),
  );
  expect(scrolls, "a card list inside the column scrolls").toBeTruthy();
  const pageScrolls = await page.evaluate(() => document.documentElement.scrollHeight > window.innerHeight + 1);
  expect(pageScrolls, "the page itself does not scroll").toBeFalsy();
});
