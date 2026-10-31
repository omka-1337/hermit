import { useEffect, useRef } from "react";

// refreshEvent asks the pages on screen to read what they show again: F5 in
// the desktop layout, as in MO2.
export const refreshEvent = "hermit:refresh";

export function requestRefresh() {
  window.dispatchEvent(new CustomEvent(refreshEvent));
}

// useRefresh runs onRefresh whenever a refresh is asked for.
export function useRefresh(onRefresh: () => void) {
  const latest = useRef(onRefresh);
  latest.current = onRefresh;
  useEffect(() => {
    const run = () => latest.current();
    window.addEventListener(refreshEvent, run);
    return () => window.removeEventListener(refreshEvent, run);
  }, []);
}
