import { useEffect, useState } from "react";

// Hermit is loading something whenever a call to its Go side is under way:
// reading profiles, mod folders and archives from disk, or the game's files.
// Every call goes to the runtime's address as a fetch, so they are counted
// here, where they all pass, rather than at each place that makes one.
// Calls quicker than delay never show: Hermit asks the game's state every
// few seconds, and a bar for that would only flicker.

const delay = 150;
// shortest is how long the bar stays once it shows, so that it is seen.
const shortest = 300;

let running = 0;
let shown = false;
let shownAt = 0;
let showTimer: ReturnType<typeof setTimeout> | null = null;
let hideTimer: ReturnType<typeof setTimeout> | null = null;
const listeners = new Set<(loading: boolean) => void>();

function set(loading: boolean) {
  if (shown === loading) return;
  shown = loading;
  if (loading) shownAt = Date.now();
  for (const listener of listeners) listener(loading);
}

function started() {
  running++;
  if (hideTimer) {
    clearTimeout(hideTimer);
    hideTimer = null;
  }
  if (!shown && !showTimer) {
    showTimer = setTimeout(() => {
      showTimer = null;
      if (running > 0) set(true);
    }, delay);
  }
}

function finished() {
  running = Math.max(0, running - 1);
  if (running > 0) return;
  if (showTimer) {
    clearTimeout(showTimer);
    showTimer = null;
  }
  if (shown && !hideTimer) {
    hideTimer = setTimeout(
      () => {
        hideTimer = null;
        if (running === 0) set(false);
      },
      Math.max(0, shortest - (Date.now() - shownAt)),
    );
  }
}

// isServiceCall tells the calls of Hermit's services, object 0 of the
// runtime, from those of its dialogs, which wait for the user, and the rest.
function isServiceCall(input: RequestInfo | URL, init?: RequestInit): boolean {
  const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
  if (!url.endsWith("/wails/runtime") || typeof init?.body !== "string") return false;
  try {
    return JSON.parse(init.body).object === 0;
  } catch {
    return false;
  }
}

const fetchWithoutCounting = window.fetch.bind(window);
window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
  if (!isServiceCall(input, init)) return fetchWithoutCounting(input, init);
  started();
  return fetchWithoutCounting(input, init).finally(finished);
};

// useLoading reports whether Hermit is loading something long enough to show.
export function useLoading(): boolean {
  const [loading, setLoading] = useState(shown);
  useEffect(() => {
    setLoading(shown);
    listeners.add(setLoading);
    return () => void listeners.delete(setLoading);
  }, []);
  return loading;
}
