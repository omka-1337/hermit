import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Application } from "@wailsio/runtime";
import { Game } from "../api";
import GameIcon from "../components/GameIcon";
import { useGamepad } from "../gamepad";
import { clickFocused, Direction, focusFirst, moveFocus } from "../focus";
import { Button, useModalOpen } from "../components/ui";
import { GearIcon, HintsContext, PlusIcon, PowerIcon, ShellProps } from "./shell";

// Entry is a stop in the game bar: a game, or one of the two actions after them.
type Entry = { kind: "game"; game: Game } | { kind: "add" } | { kind: "settings" };

// Zone is where the buttons act: on the bar of games, or inside the page below
// it. The bar shrinks out of the way while the page has the focus.
type Zone = "bar" | "page";

// ConsoleShell is the Steam Deck chrome: games in a row along the top, one page
// per entry, switched with the shoulder buttons like Steam's Big Picture.
export default function ConsoleShell({
  games,
  selectedId,
  page,
  canGoBack,
  onOpenGame,
  onAdd,
  onSettings,
  onBack,
  children,
}: ShellProps) {
  const entries: Entry[] = [
    ...games.map((game): Entry => ({ kind: "game", game })),
    { kind: "add" },
    { kind: "settings" },
  ];
  const current =
    page === "settings"
      ? entries.length - 1
      : page === "add"
        ? entries.length - 2
        : entries.findIndex((e) => e.kind === "game" && e.game.id === selectedId);
  const [index, setIndex] = useState(Math.max(current, 0));
  const [zone, setZone] = useState<Zone>("bar");
  const main = useRef<HTMLElement>(null);
  // Set once the user moves the focus themselves, so a page that is still
  // loading stops pulling it back.
  const moved = useRef(false);

  // Follow page changes made elsewhere, e.g. right after a game is added.
  useEffect(() => {
    if (current >= 0) setIndex(current);
  }, [current]);

  // Switching pages while the focus is inside one lands on the new page's own
  // control instead of nowhere. Pages load their contents after they mount, so
  // the aim is corrected as the real controls (a game's profiles, say) arrive.
  useEffect(() => {
    if (zone !== "page") return;
    if (!focusFirst(main.current)) {
      setZone("bar");
      return;
    }
    moved.current = false;
    const observer = new MutationObserver(() => {
      if (moved.current) observer.disconnect();
      else focusFirst(main.current);
    });
    if (main.current) observer.observe(main.current, { childList: true, subtree: true });
    const stop = setTimeout(() => observer.disconnect(), 2000);
    return () => {
      observer.disconnect();
      clearTimeout(stop);
    };
  }, [zone, page, selectedId]);

  const enterPage = () => {
    if (focusFirst(main.current)) setZone("page");
  };

  const move = (dir: Direction) => {
    if (moveFocus(dir, main.current)) moved.current = true;
  };

  const backToBar = () => {
    (document.activeElement as HTMLElement | null)?.blur();
    setZone("bar");
  };

  // Every entry is a page, so moving the highlight switches the view at once.
  const goto = (to: number) => {
    const wrapped = (to + entries.length) % entries.length;
    setIndex(wrapped);
    const entry = entries[wrapped];
    if (entry.kind === "game") onOpenGame(entry.game.id);
    if (entry.kind === "add") onAdd();
    if (entry.kind === "settings") onSettings();
  };

  const [leaving, setLeaving] = useState(false);
  const [pageHints, setPageHints] = useState<string[]>([]);
  const publishHints = useCallback((hints: string[]) => setPageHints(hints), []);

  // A dialog takes over the buttons while it is open.
  const modalOpen = useModalOpen();
  useGamepad(
    {
      // The shoulders switch games from anywhere, the d-pad stays in its zone.
      onPrev: () => goto(index - 1),
      onNext: () => goto(index + 1),
      onLeft: () => (zone === "bar" ? goto(index - 1) : move("left")),
      onRight: () => (zone === "bar" ? goto(index + 1) : move("right")),
      onDown: () => (zone === "bar" ? enterPage() : move("down")),
      // Up at the top of a page returns to the games.
      onUp: () => {
        if (zone !== "page") return;
        if (moveFocus("up", main.current)) moved.current = true;
        else backToBar();
      },
      onAccept: () => {
        if (leaving) Application.Quit();
        else if (zone === "page") clickFocused(main.current);
        else enterPage();
      },
      // B is the way back: out of the page to the bar, then out of whatever the
      // bar is showing, and finally out of Hermit.
      onBack: () => {
        if (leaving) setLeaving(false);
        else if (zone === "page") backToBar();
        else if (canGoBack) onBack();
        else setLeaving(true);
      },
    },
    !modalOpen,
  );

  return (
    <div className="flex h-full flex-col">
      <GameBar entries={entries} index={index} compact={zone === "page"} onPick={goto} />
      <main ref={main} className="min-h-0 flex-1 overflow-auto">
        <HintsContext.Provider value={publishHints}>{children}</HintsContext.Provider>
      </main>
      <HintBar zone={zone} canGoBack={canGoBack} pageHints={pageHints} />
      {leaving && <QuitPrompt onCancel={() => setLeaving(false)} onQuit={() => Application.Quit()} />}
    </div>
  );
}

type GameBarProps = {
  entries: Entry[];
  index: number;
  // compact shrinks the bar to icons once the page below has the focus.
  compact: boolean;
  onPick: (index: number) => void;
};

// GameBar keeps the selected entry in the middle: the strip slides under it.
function GameBar({ entries, index, compact, onPick }: GameBarProps) {
  const viewport = useRef<HTMLDivElement>(null);
  const strip = useRef<HTMLDivElement>(null);
  const [offset, setOffset] = useState(0);

  useLayoutEffect(() => {
    const center = () => {
      const box = viewport.current;
      const tile = strip.current?.children[index] as HTMLElement | undefined;
      if (!box || !tile) return;
      setOffset(box.clientWidth / 2 - (tile.offsetLeft + tile.offsetWidth / 2));
    };
    center();
    // The window can be resized (and the deck rotates its own scale).
    const observer = new ResizeObserver(center);
    if (viewport.current) observer.observe(viewport.current);
    return () => observer.disconnect();
  }, [index, entries.length, compact]);

  return (
    <header
      className={`flex items-center gap-3 border-b border-zinc-800 bg-zinc-900 px-3 transition-all duration-200 ${
        compact ? "py-1" : "py-2"
      }`}
    >
      <ShoulderHint label="LB" />
      {/* The vertical padding keeps the selection ring from being clipped. */}
      <div ref={viewport} className="relative -my-1 min-w-0 flex-1 overflow-hidden py-1">
        <div
          ref={strip}
          className="flex gap-2 transition-transform duration-200 ease-out"
          style={{ transform: `translateX(${offset}px)` }}
        >
          {entries.map((entry, i) => {
            const active = i === index;
            const tile = `flex shrink-0 flex-col items-center justify-center gap-1 rounded-xl text-xs transition-all duration-200 ${
              compact ? "h-11 w-11 px-0" : "h-[5rem] w-28 px-2"
            } ${active ? "bg-zinc-800 text-white ring-2 ring-accent-500" : "text-zinc-400 opacity-70"}`;
            if (entry.kind === "game") {
              return (
                <button key={entry.game.id} onClick={() => onPick(i)} className={tile} title={entry.game.name}>
                  <GameIcon name={entry.game.name} steamAppId={entry.game.steamAppId} size={compact ? 28 : 36} />
                  {!compact && (
                    <span className="line-clamp-2 w-full text-center leading-tight break-words">{entry.game.name}</span>
                  )}
                </button>
              );
            }
            if (entry.kind === "add") {
              return (
                <button
                  key="add"
                  onClick={() => onPick(i)}
                  title="Add game"
                  className={`${tile} border border-dashed border-zinc-700`}
                >
                  <PlusIcon />
                  {!compact && "Add game"}
                </button>
              );
            }
            return (
              <button key="settings" onClick={() => onPick(i)} className={tile} title="Settings">
                <GearIcon />
                {!compact && "Settings"}
              </button>
            );
          })}
        </div>
      </div>
      <ShoulderHint label="RB" />
    </header>
  );
}

function ShoulderHint({ label }: { label: string }) {
  return (
    <span className="shrink-0 rounded-md border border-zinc-700 px-2 py-1 text-xs font-semibold text-zinc-400">
      {label}
    </span>
  );
}

function HintBar({ zone, canGoBack, pageHints }: { zone: Zone; canGoBack: boolean; pageHints: string[] }) {
  const hints =
    zone === "bar"
      ? ["LB / RB — switch", "A / ↓ — open", canGoBack ? "B — back" : "B — quit"]
      : ["D-pad — move", "A — select", "B — back to games", ...pageHints];
  return (
    <footer className="flex gap-4 border-t border-zinc-800 bg-zinc-900 px-4 py-1.5 text-xs text-zinc-500">
      {hints.map((hint) => (
        <span key={hint}>{hint}</span>
      ))}
    </footer>
  );
}

// QuitPrompt is the last step of pressing B: it is drawn here rather than in a
// Modal so the controller keeps working while it is up (A quits, B cancels).
function QuitPrompt({ onCancel, onQuit }: { onCancel: () => void; onQuit: () => void }) {
  return (
    <div className="fixed inset-0 z-20 flex items-center justify-center bg-black/70 p-6" onClick={onCancel}>
      <div
        className="flex w-full max-w-sm flex-col gap-4 rounded-xl border border-zinc-700 bg-zinc-900 p-6 text-center"
        onClick={(e) => e.stopPropagation()}
      >
        <span className="self-center text-zinc-500">
          <PowerIcon />
        </span>
        <div className="flex flex-col gap-1">
          <span className="text-lg font-semibold">Quit Hermit?</span>
          <span className="text-sm text-zinc-400">Games keep their mods; nothing is removed.</span>
        </div>
        <div className="flex justify-center gap-2">
          <Button variant="primary" onClick={onQuit}>
            Quit (A)
          </Button>
          <Button variant="ghost" onClick={onCancel}>
            Cancel (B)
          </Button>
        </div>
      </div>
    </div>
  );
}
