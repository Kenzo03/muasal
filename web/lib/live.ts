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

// Source is the part of EventSource the tab's stream uses.
export type Source = {
  readyState: number;
  onopen: ((e: never) => unknown) | null;
  onerror: ((e: never) => unknown) | null;
  addEventListener(type: string, f: (e: { data: string }) => void): void;
  close(): void;
};

const CLOSED = 2; // EventSource.CLOSED

// liveStream keeps the tab's event stream open (spec: live ticket updates).
// EventSource retries a dropped connection by itself but gives up for good on
// a non-200 answer, such as Caddy's 502 while the app restarts; then this
// opens a new one after 5 s, doubling to at most 60 s while it keeps failing.
// Every open after the first calls reopened, since changes may have been
// missed meanwhile. It returns stop.
export function liveStream(
  open: () => Source,
  types: string[],
  on: { reopened(): void; event(type: string, data: string): void },
  clock: Clock = realClock,
): () => void {
  let source: Source;
  let opened = false;
  let failures = 0;
  let timer: unknown;
  let stopped = false;
  const connect = () => {
    timer = undefined;
    source = open();
    source.onopen = () => {
      if (opened) on.reopened();
      opened = true;
      failures = 0;
    };
    source.onerror = () => {
      if (source.readyState !== CLOSED || stopped) return;
      source.close();
      timer = clock.later(connect, Math.min(60_000, 5_000 * 2 ** failures));
      failures++;
    };
    for (const type of types) source.addEventListener(type, (e) => on.event(type, e.data));
  };
  connect();
  return () => {
    stopped = true;
    if (timer !== undefined) clock.cancel(timer);
    source.close();
  };
}
