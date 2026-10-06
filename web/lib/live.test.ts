import assert from "node:assert/strict";
import test from "node:test";
import { liveStream, refresher, type Clock } from "./live.ts";

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

// A stand-in EventSource the test drives.
class FakeSource {
  static made: FakeSource[] = [];
  readyState = 0;
  closed = false;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  listeners = new Map<string, (e: { data: string }) => void>();
  constructor() {
    FakeSource.made.push(this);
  }
  addEventListener(type: string, f: (e: { data: string }) => void) {
    this.listeners.set(type, f);
  }
  close() {
    this.closed = true;
    this.readyState = 2;
  }
  open() {
    this.readyState = 1;
    this.onopen?.();
  }
  fail(gaveUp: boolean) {
    this.readyState = gaveUp ? 2 : 0;
    this.onerror?.();
  }
}

function watch() {
  FakeSource.made = [];
  const { clock, tick } = fakeClock();
  const log: string[] = [];
  const stop = liveStream(() => new FakeSource(), ["tickets"], { reopened: () => log.push("reopened"), event: (type, data) => log.push(`${type}:${data}`) }, clock);
  return { clock, tick, log, stop };
}

// Review #2: EventSource retries a dropped connection itself, but gives up for
// good on a non-200 answer (Caddy's 502 while the app restarts).
test("a stream that gave up is reopened after a delay, and counts as a reopen", () => {
  const { tick, log } = watch();
  FakeSource.made[0].open();
  assert.deepEqual(log, []);
  FakeSource.made[0].fail(true);
  assert.equal(FakeSource.made.length, 1);
  tick(5_000);
  assert.equal(FakeSource.made.length, 2);
  FakeSource.made[1].open();
  assert.deepEqual(log, ["reopened"]);
});

test("the delay grows while reopening keeps failing, then resets", () => {
  const { tick } = watch();
  FakeSource.made[0].fail(true);
  tick(5_000);
  FakeSource.made[1].fail(true);
  tick(5_000);
  assert.equal(FakeSource.made.length, 2); // now waits 10 s
  tick(5_000);
  assert.equal(FakeSource.made.length, 3);
  FakeSource.made[2].open();
  FakeSource.made[2].fail(true);
  tick(5_000);
  assert.equal(FakeSource.made.length, 4); // back to 5 s after a good open
});

test("the browser's own retry counts as a reopen too", () => {
  const { log } = watch();
  FakeSource.made[0].open();
  FakeSource.made[0].fail(false);
  FakeSource.made[0].open();
  assert.equal(FakeSource.made.length, 1);
  assert.deepEqual(log, ["reopened"]);
});

test("events pass through, and stop closes and cancels a pending reopen", () => {
  const { tick, log, stop } = watch();
  FakeSource.made[0].listeners.get("tickets")!({ data: '{"tickets":[7]}' });
  assert.deepEqual(log, ['tickets:{"tickets":[7]}']);
  FakeSource.made[0].fail(true);
  stop();
  tick(60_000);
  assert.equal(FakeSource.made.length, 1);
  assert.ok(FakeSource.made[0].closed);
});
