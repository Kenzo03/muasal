// The refresh rules for live updates (spec: live ticket updates): a change
// refreshes the page at once, then at most once per gap; while the page is
// paused (an editor is open) or hidden, changes wait and run once.
export type Clock = { now(): number; later(f: () => void, ms: number): unknown; cancel(id: unknown): void };

export type Refresher = { signal(): void; pause(on: boolean): void; visible(on: boolean): void; dispose(): void };

const realClock: Clock = {
  now: () => Date.now(),
  later: (f, ms) => setTimeout(f, ms),
  cancel: (id) => clearTimeout(id as ReturnType<typeof setTimeout>),
};

export function refresher(refresh: () => void, gapMs = 500, clock: Clock = realClock): Refresher {
  let pending = false;
  let paused = false;
  let hidden = false;
  let last = -Infinity;
  let timer: unknown;
  const run = () => {
    timer = undefined;
    if (!pending || paused || hidden) return;
    const wait = last + gapMs - clock.now();
    if (wait > 0) {
      timer = clock.later(run, wait);
      return;
    }
    pending = false;
    last = clock.now();
    refresh();
  };
  const kick = () => {
    if (timer === undefined) run();
  };
  return {
    signal() {
      pending = true;
      kick();
    },
    pause(on) {
      paused = on;
      if (!on) kick();
    },
    visible(on) {
      hidden = !on;
      if (on) kick();
    },
    dispose() {
      if (timer !== undefined) clock.cancel(timer);
      timer = undefined;
    },
  };
}
