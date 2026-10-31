import { CSSProperties, useEffect, useRef } from "react";
import { Events, Window } from "@wailsio/runtime";
import AddToSteam from "../components/AddToSteam";

// Wails starts a window drag from elements carrying this variable.
const drag = { "--wails-draggable": "drag" } as CSSProperties;
const noDrag = { "--wails-draggable": "no-drag" } as CSSProperties;

// The main window is waiting for this; it opens once this one is done.
const doneEvent = "steam-setup:done";

// Kept in step with the width the Go side opens this window with.
const windowWidth = 520;

// SteamSetupWindow is the small window shown on the very first run, before
// the manager itself. It has its own frame because it is a separate window,
// not a screen inside the app.
export default function SteamSetupWindow() {
  const content = useRef<HTMLElement>(null);

  // The text changes with the state of Steam, so the window is sized to what
  // is actually in it instead of leaving a gap under the buttons.
  useEffect(() => {
    const box = content.current;
    if (!box) return;
    const fit = () => {
      const height = Math.ceil(box.scrollHeight + (box.getBoundingClientRect().top || 0));
      Window.SetSize(windowWidth, Math.min(Math.max(height, 220), 640)).catch(() => {});
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(box);
    return () => observer.disconnect();
  }, []);

  const done = () => {
    Events.Emit(doneEvent);
    // The main process closes this window; closing it here too keeps the
    // click responsive if that event is ever missed.
    Window.Close();
  };

  return (
    <div className="flex h-full flex-col bg-zinc-950">
      <header
        style={drag}
        className="flex h-9 shrink-0 items-center justify-between border-b border-zinc-800 bg-zinc-900 select-none"
      >
        <span className="px-4 text-xs font-medium text-zinc-400">Hermit setup</span>
        <button
          style={noDrag}
          aria-label="Close"
          onClick={done}
          className="flex h-full w-11 items-center justify-center text-zinc-400 transition-colors hover:bg-red-600 hover:text-white"
        >
          <svg viewBox="0 0 24 24" className="h-3.5 w-3.5" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M6 6l12 12M18 6 6 18" strokeLinecap="round" />
          </svg>
        </button>
      </header>

      <main ref={content} className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-6">
        <div className="flex flex-col gap-1">
          <h1 className="text-xl font-semibold">Welcome to Hermit</h1>
          <p className="text-sm text-zinc-400">
            A mod manager for BepInEx games. Mods live in profiles of their own and are linked into a game only while
            it runs, so game folders stay untouched.
          </p>
        </div>
        <AddToSteam onDone={done} skipLabel="Not now" />
      </main>
    </div>
  );
}
