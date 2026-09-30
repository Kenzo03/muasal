import assert from "node:assert/strict";
import test from "node:test";
import { toggleTask } from "./tasks.ts";

// MSL-55: ticking a step changes only its own line.
test("ticks one task line", () => {
  const md = "Langkah:\n- [ ] Salin jadwal\n  * [x] Kirim ke toko\n1. [ ] Uji\nTeks [ ] biasa";
  assert.equal(toggleTask(md, 2, true), "Langkah:\n- [x] Salin jadwal\n  * [x] Kirim ke toko\n1. [ ] Uji\nTeks [ ] biasa");
  assert.equal(toggleTask(md, 3, false), "Langkah:\n- [ ] Salin jadwal\n  * [ ] Kirim ke toko\n1. [ ] Uji\nTeks [ ] biasa");
  assert.equal(toggleTask(md, 4, true).split("\n")[3], "1. [x] Uji");
  assert.equal(toggleTask(md, 5, true), md); // not a task line
  assert.equal(toggleTask(md, 9, true), md);
});
