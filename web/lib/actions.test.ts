import assert from "node:assert/strict";
import test from "node:test";
import { actionItems } from "./actions.ts";

// MSL-11: the action items of the MJD meeting note in the walkthrough.
test("reads a note's action items", () => {
  const body = `## Keputusan

1. Untuk pelanggan bertanda prioritas, batas kredit hanya memberi peringatan.

## Tindak lanjut

- Bayu: rancang penanda pelanggan prioritas di menu Pelanggan, paling lambat 10 Oktober 2026.
- Hendra: kirim daftar pelanggan prioritas per cabang, paling lambat 30 Sep.
- Dewi: perbarui manual pengguna Batas Kredit setelah rilis.
* Siapkan data uji (by 2026-10-05)

## Catatan

- Bukan tindak lanjut.`;
  assert.deepEqual(actionItems(body, 2026), [
    { owner: "Bayu", task: "rancang penanda pelanggan prioritas di menu Pelanggan", due: "2026-10-10" },
    { owner: "Hendra", task: "kirim daftar pelanggan prioritas per cabang", due: "2026-09-30" },
    { owner: "Dewi", task: "perbarui manual pengguna Batas Kredit setelah rilis", due: undefined },
    { owner: undefined, task: "Siapkan data uji", due: "2026-10-05" },
  ]);
  assert.deepEqual(actionItems("## Action items\n\n1. Rina: send the minutes by 2 Oct 2026", 2026), [
    { owner: "Rina", task: "send the minutes", due: "2026-10-02" },
  ]);
  assert.deepEqual(actionItems("## Keputusan\n\n- Tidak ada tindak lanjut di sini.", 2026), []);
});
