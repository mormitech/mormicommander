// Waiting for the end of a job that Go runs in the background and announces with a "done" event (the server transfers
// of features/net.js). Go starts the job before the call that starts it returns, so a job that takes only a few
// milliseconds (a rename on the same server, a tiny upload) can send its "done" event before the page knows the job's
// number. Such an early end is kept here, and wait() picks it up at once instead of waiting forever (board issue #140).
//
//   const w = jobWaiter();
//   on('net-job-done', (p) => w.done(p));
//   const final = await w.wait(id, hooks); // hooks: {update(p), close()}; get(id) gives the hooks of a running job

const KEEP_EARLY = 50; // ends of jobs nobody waits for (yet) that are kept; the oldest go first

export function jobWaiter() {
  const waiting = new Map(); // id → {resolve, hooks}
  const early = new Map(); // id → final progress that came before wait(id)
  return {
    // done takes a "done" event: it resolves the job's wait, or keeps the end for a wait that comes later.
    done(p) {
      const w = waiting.get(p.id);
      if (!w) {
        early.set(p.id, p);
        while (early.size > KEEP_EARLY) early.delete(early.keys().next().value);
        return;
      }
      waiting.delete(p.id);
      w.hooks.close?.();
      w.resolve(p);
    },
    // get is the hooks of a job that is being waited for (for progress events), or undefined.
    get(id) { return waiting.get(id)?.hooks; },
    // wait resolves with the job's final progress; at once when the job has already ended.
    wait(id, hooks = {}) {
      return new Promise((resolve) => {
        if (early.has(id)) { const p = early.get(id); early.delete(id); resolve(p); return; }
        waiting.set(id, { resolve, hooks });
      });
    },
  };
}
