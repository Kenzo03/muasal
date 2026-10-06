import assert from "node:assert/strict";
import test from "node:test";
import { refresher, type Clock } from "./live.ts";

// A clock the test drives: later() queues, tick(ms) runs what is due.
function fakeClock() {
  let t = 0;
  let q: { at: number; f: () => void; id: number }[] = [];
  let n = 0;
  const clock: Clock = {
    now: () => t,
    later: (f, ms) => {
      q.push({ at: t + ms, f, id: ++n });
      return n;
    },
    cancel: (id) => {
      q = q.filter((x) => x.id !== id);
    },
  };
  const tick = (ms: number) => {
    t += ms;
    for (let due = q.filter((x) => x.at <= t); due.length; due = q.filter((x) => x.at <= t)) {
      q = q.filter((x) => x.at > t);
      due.forEach((x) => x.f());
    }
  };
  return { clock, tick };
}

test("refreshes at once, then at most once per gap", () => {
  const { clock, tick } = fakeClock();
  let runs = 0;
  const r = refresher(() => runs++, 500, clock);
  r.signal();
  assert.equal(runs, 1);
  r.signal();
  r.signal();
  assert.equal(runs, 1);
  tick(499);
  assert.equal(runs, 1);
  tick(1);
  assert.equal(runs, 2); // the two later signals run once
  tick(1000);
  assert.equal(runs, 2);
});

test("holds while paused and runs once on resume", () => {
  const { clock } = fakeClock();
  let runs = 0;
  const r = refresher(() => runs++, 500, clock);
  r.pause(true);
  r.signal();
  r.signal();
  assert.equal(runs, 0);
  r.pause(false);
  assert.equal(runs, 1);
});

test("a hidden tab refreshes exactly once when shown", () => {
  const { clock, tick } = fakeClock();
  let runs = 0;
  const r = refresher(() => runs++, 500, clock);
  r.visible(false);
  r.signal();
  tick(2000);
  r.signal();
  assert.equal(runs, 0);
  r.visible(true);
  assert.equal(runs, 1);
  tick(2000);
  assert.equal(runs, 1);
});

test("nothing pending: resume and show do not refresh", () => {
  const { clock } = fakeClock();
  let runs = 0;
  const r = refresher(() => runs++, 500, clock);
  r.pause(true);
  r.pause(false);
  r.visible(false);
  r.visible(true);
  assert.equal(runs, 0);
});
