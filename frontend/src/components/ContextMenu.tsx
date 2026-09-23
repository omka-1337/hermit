import { ReactNode, useEffect, useLayoutEffect, useRef, useState } from "react";
import { clickFocused, focusFirst, moveFocus } from "../focus";
import { useGamepad } from "../gamepad";
import { useModalLayer } from "../modals";
import { useLayout } from "../uimode";

export type MenuAnchor = { x: number; y: number };

type Props = {
  at: MenuAnchor;
  onClose: () => void;
  children: ReactNode;
};

// ContextMenu is the little panel a right click opens, keeping the rarely used
// and the destructive off the card itself. It is driven by the controller as
// well: the d-pad walks the items, A picks one and B closes it.
export default function ContextMenu({ at, onClose, children }: Props) {
  const deck = useLayout() === "deck";
  const box = useRef<HTMLDivElement>(null);
  const top = useModalLayer(true);
  const [pos, setPos] = useState(at);

  // Keep the whole menu on screen, wherever it was opened.
  useLayoutEffect(() => {
    const menu = box.current;
    if (!menu) return;
    const { width, height } = menu.getBoundingClientRect();
    setPos({
      x: Math.max(8, Math.min(at.x, window.innerWidth - width - 8)),
      y: Math.max(8, Math.min(at.y, window.innerHeight - height - 8)),
    });
  }, [at]);

  useEffect(() => {
    if (deck && top) focusFirst(box.current);
  }, [deck, top]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  useGamepad(
    {
      onUp: () => moveFocus("up", box.current),
      onDown: () => moveFocus("down", box.current),
      onAccept: () => clickFocused(box.current),
      onBack: onClose,
    },
    deck && top,
  );

  return (
    <div className="fixed inset-0 z-30" onMouseDown={onClose} onContextMenu={(e) => e.preventDefault()}>
      <div
        ref={box}
        style={{ left: pos.x, top: pos.y }}
        className="fixed flex w-52 flex-col rounded-lg border border-zinc-700 bg-zinc-900 p-1 shadow-xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}

type ItemProps = {
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
  title?: string;
  children: ReactNode;
};

export function MenuItem({ onClick, disabled, danger, title, children }: ItemProps) {
  return (
    <button
      disabled={disabled}
      title={title}
      onClick={onClick}
      className={`rounded px-3 py-2 text-left text-sm transition-colors disabled:pointer-events-none disabled:opacity-40 ${
        danger ? "text-red-400 hover:bg-red-500/10" : "text-zinc-200 hover:bg-zinc-800"
      }`}
    >
      {children}
    </button>
  );
}
