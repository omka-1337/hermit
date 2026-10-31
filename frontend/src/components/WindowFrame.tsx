import { CSSProperties, ReactNode, useEffect, useState } from "react";
import { Window } from "@wailsio/runtime";
import { useLayout } from "../uimode";
import TitleMenus from "./TitleMenus";

// Wails starts a window drag from elements with this CSS variable (frameless windows).
const drag = { "--wails-draggable": "drag" } as CSSProperties;
const noDrag = { "--wails-draggable": "no-drag" } as CSSProperties;

function useMaximised(): boolean {
  const [maximised, setMaximised] = useState(false);
  useEffect(() => {
    const check = () => Window.IsMaximised().then(setMaximised).catch(() => {});
    check();
    window.addEventListener("resize", check);
    return () => window.removeEventListener("resize", check);
  }, []);
  return maximised;
}

// WindowFrame replaces the system window decorations: a title bar with window
// controls and a thin border so the window edge stays visible on dark desktops.
// The deck layout runs full screen like a console, so it has no title bar at
// all; quitting lives in its bar of games.
//
// The main window has the Hermit and File menus in its title bar; another
// window, an installer's, has a title instead, and onClose runs before it
// closes.
export default function WindowFrame({
  children,
  menus = false,
  title = "Hermit",
  onClose,
}: {
  children: ReactNode;
  menus?: boolean;
  title?: string;
  onClose?: () => void;
}) {
  const maximised = useMaximised();
  const deck = useLayout() === "deck";

  if (deck) return <div className="h-full bg-zinc-950">{children}</div>;

  return (
    <div className={`flex h-full flex-col bg-zinc-950 ${maximised ? "" : "border border-zinc-700/70"}`}>
      <header
        style={drag}
        onDoubleClick={() => Window.ToggleMaximise()}
        className="flex h-9 shrink-0 items-center border-b border-zinc-800 bg-zinc-900 select-none"
      >
        {menus ? <TitleMenus /> : <span className="truncate px-4 text-xs font-medium text-zinc-400">{title}</span>}
        <div className="flex-1" />
        <div style={noDrag} className="flex h-full" onDoubleClick={(e) => e.stopPropagation()}>
          <WindowButton label="Minimise" onClick={() => Window.Minimise()}>
            <path d="M5 12h14" />
          </WindowButton>
          <WindowButton label={maximised ? "Restore" : "Maximise"} onClick={() => Window.ToggleMaximise()}>
            {maximised ? (
              <>
                <rect x="5" y="9" width="10" height="10" rx="1" />
                <path d="M9 9V6a1 1 0 0 1 1-1h8a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1h-3" />
              </>
            ) : (
              <rect x="5" y="5" width="14" height="14" rx="1" />
            )}
          </WindowButton>
          <WindowButton
            label="Close"
            danger
            onClick={() => {
              onClose?.();
              Window.Close();
            }}
          >
            <path d="M6 6l12 12M18 6 6 18" />
          </WindowButton>
        </div>
      </header>
      <div className="min-h-0 flex-1">{children}</div>
    </div>
  );
}

type ButtonProps = {
  label: string;
  danger?: boolean;
  onClick: () => void;
  children: ReactNode;
};

function WindowButton({ label, danger, onClick, children }: ButtonProps) {
  return (
    <button
      aria-label={label}
      title={label}
      onClick={onClick}
      className={`flex w-11 items-center justify-center text-zinc-400 transition-colors ${
        danger ? "hover:bg-red-600 hover:text-white" : "hover:bg-zinc-800 hover:text-zinc-100"
      }`}
    >
      <svg
        viewBox="0 0 24 24"
        className="h-3.5 w-3.5"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        {children}
      </svg>
    </button>
  );
}
