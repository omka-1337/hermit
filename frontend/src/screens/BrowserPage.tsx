import { useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { BrowserService, BrowserState, errorMessage } from "../api";
import { ErrorText, useModalOpen } from "../components/ui";

// openBrowserEvent asks the window to show a page of Nexus Mods in its
// browser, with the address as its detail.
export const openBrowserEvent = "hermit:open-browser";

// openInBrowser shows a page of Nexus Mods inside Hermit.
export function openInBrowser(url: string) {
  window.dispatchEvent(new CustomEvent(openBrowserEvent, { detail: url }));
}

type Props = {
  url: string;
  onClose: () => void;
};

// BrowserPage shows a page of Nexus Mods where Hermit's pages go, in a
// browser of its own the window lays over this page's area: it shares
// nothing with Hermit's pages. Over it are Back, Reload, where it is and a
// way out; its "Mod Manager Download" hands the file to Hermit, which then
// closes the page. Hermit's dialogs and menus would be drawn under the
// browser, so it steps aside while one is open.
export default function BrowserPage({ url, onClose }: Props) {
  const area = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<BrowserState | null>(null);
  const [error, setError] = useState("");
  const modalOpen = useModalOpen();

  // place puts the browser over the area; it is asked for again once the
  // page has opened, as the first page makes the browser.
  const place = useRef(() => {});
  useEffect(() => {
    setError("");
    BrowserService.Open(url)
      .then(() => place.current())
      .catch((err) => setError(errorMessage(err)));
  }, [url]);

  // crashed says why the page is gone, until it loads again.
  const [crashed, setCrashed] = useState("");
  useEffect(() => {
    const offChanged = Events.On("browser:changed", (ev) => {
      const next = ev.data as BrowserState;
      setState(next);
      if (next.loading) setCrashed("");
    });
    const offCrashed = Events.On("browser:crashed", (ev) => setCrashed(String(ev.data)));
    return () => {
      offChanged();
      offCrashed();
    };
  }, []);

  // The browser follows the area wherever the window and the sidebar move it.
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    if (modalOpen) {
      BrowserService.Hide();
      return;
    }
    const put = () => {
      const r = el.getBoundingClientRect();
      BrowserService.Place(r.left, r.top, r.width, r.height);
    };
    place.current = put;
    put();
    const observer = new ResizeObserver(put);
    observer.observe(el);
    window.addEventListener("resize", put);
    return () => {
      place.current = () => {};
      observer.disconnect();
      window.removeEventListener("resize", put);
    };
  }, [modalOpen]);

  useEffect(
    () => () => {
      BrowserService.Hide();
    },
    [],
  );

  const address = state?.uri || url;
  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-1 border-b border-zinc-800 px-3 py-2">
        <ToolButton title="Back" disabled={!state?.canGoBack} onClick={() => BrowserService.Back()}>
          <path d="m15 18-6-6 6-6" />
        </ToolButton>
        <ToolButton title="Reload" onClick={() => BrowserService.Reload()}>
          <path d="M20 11a8 8 0 1 0-2.3 5.7" />
          <path d="M20 4v7h-7" />
        </ToolButton>
        <div className="mx-2 flex min-w-0 flex-1 flex-col">
          <span className="truncate text-sm font-medium">{state?.title || "Nexus Mods"}</span>
          <span className="truncate text-xs text-zinc-500" title={address}>
            {address}
          </span>
        </div>
        <button
          onClick={onClose}
          className="flex shrink-0 items-center gap-2 rounded-md border border-zinc-700 px-3 py-1.5 text-sm text-zinc-200 hover:bg-zinc-800"
        >
          <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden>
            <path d="M18 6 6 18M6 6l12 12" />
          </svg>
          Close
        </button>
      </div>
      <div className="relative h-0.5 shrink-0 bg-transparent">
        {state?.loading && (
          <div className="absolute inset-y-0 left-0 bg-accent-500 transition-[width]" style={{ width: `${Math.round((state.progress || 0.05) * 100)}%` }} />
        )}
      </div>
      {error && (
        <div className="p-6">
          <ErrorText>{error}</ErrorText>
        </div>
      )}
      {crashed && (
        <div className="flex shrink-0 items-center gap-3 border-b border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-300">
          <span className="flex-1">Nexus Mods could not be shown: {crashed}.</span>
          <button onClick={() => BrowserService.Reload()} className="rounded-md border border-red-500/40 px-3 py-1 hover:bg-red-500/20">
            Reload
          </button>
        </div>
      )}
      {/* The browser is laid over this area. */}
      <div ref={area} className="min-h-0 flex-1 bg-zinc-900" />
    </div>
  );
}

function ToolButton({
  title,
  disabled,
  onClick,
  children,
}: {
  title: string;
  disabled?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      title={title}
      aria-label={title}
      disabled={disabled}
      onClick={onClick}
      className="flex h-8 w-8 items-center justify-center rounded-md text-zinc-300 hover:bg-zinc-800 disabled:opacity-30 disabled:hover:bg-transparent"
    >
      <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
        {children}
      </svg>
    </button>
  );
}
