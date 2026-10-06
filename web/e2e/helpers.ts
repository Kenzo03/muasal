import { expect, test, type Locator, type Page } from "@playwright/test";

// The tests run in the default UI language, Indonesian: after sign-in the UI
// follows the user's profile language, and new users start with `id`.

export async function setPassword(page: Page, link: string, password: string) {
  for (let tries = 0; ; tries++) {
    await page.goto(link);
    await page.getByLabel("Kata sandi baru").fill(password);
    await page.getByLabel("Ulangi kata sandi").fill(password);
    const answer = page.waitForResponse((r) => r.url().endsWith("/auth/setup"));
    await page.getByRole("button", { name: "Simpan kata sandi" }).click();
    const res = await answer;
    // The setup-link endpoints share a limit of 20 requests a minute per
    // address, and a full run opens more links than that, so a limited attempt
    // waits for the next minute and tries again, up to three times: parallel
    // specs can fill the next minute too.
    if (tries < 3 && res.status() === 429 && (await res.json()).code === "rate_limited") {
      test.info().setTimeout(test.info().timeout + 65_000);
      await page.waitForTimeout(61_000);
      continue;
    }
    await expect(page.getByRole("status")).toHaveText("Kata sandi tersimpan. Silakan masuk.");
    return;
  }
}

export async function signIn(page: Page, email: string, password: string) {
  for (let tries = 0; ; tries++) {
    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Kata sandi").fill(password);
    const answer = page.waitForResponse((r) => r.url().endsWith("/auth/login"));
    await page.getByRole("button", { name: "Masuk" }).click();
    const res = await answer;
    // The server takes 20 sign-ins a minute from one address (FSD §15.1), and a
    // full run signs in more often than that, so a limited attempt waits for the
    // next minute and tries once more.
    if (tries === 0 && res.status() === 429 && (await res.json()).code === "rate_limited") {
      test.info().setTimeout(test.info().timeout + 65_000);
      await page.waitForTimeout(61_000);
      continue;
    }
    await expect(page).toHaveURL(/\/$/);
    return;
  }
}

// Drags with several pointer moves, as dnd-kit waits for a short move before a
// drag starts. The grab point sits near the top-left corner, clear of links and
// menus inside the source; `to` picks the drop point as a share of the target's box.
export async function drag(page: Page, source: Locator, target: Locator, to = { x: 0.5, y: 0.5 }) {
  const from = await source.boundingBox();
  const box = await target.boundingBox();
  if (!from || !box) throw new Error("drag: source or target is not visible");
  await page.mouse.move(from.x + 8, from.y + 8);
  await page.mouse.down();
  await page.mouse.move(from.x + 20, from.y + 20, { steps: 4 });
  await page.mouse.move(box.x + box.width * to.x, box.y + box.height * to.y, { steps: 12 });
  await page.mouse.up();
  // dnd-kit stops every click for 50 ms after a drop, so a click on what the drop
  // opens (the close dialog's Batal) can be lost on a fast server. Wait it out.
  await page.waitForTimeout(100);
}
