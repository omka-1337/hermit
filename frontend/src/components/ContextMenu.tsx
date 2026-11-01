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
  // width is the panel's width class; a menu opened from a button in the
  // sidebar takes the sidebar's.
  width?: string;
};

// ContextMenu is the little panel a right click opens, keeping the rarely used
// and the destructive off the card itself. It is driven by the controller as
// well: the d-pad walks the items, A picks one and B closes it.
export default function ContextMenu({ at, onClose, children, width = "w-52" }: Props) {
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
    <div className="fixed inset-0 z-40" onMouseDown={onClose} onContextMenu={(e) => e.preventDefault()}>
      <div
        ref={box}
        style={{ left: pos.x, top: pos.y }}
        className={`fixed flex max-h-[80vh] flex-col overflow-y-auto rounded-lg border border-zinc-700 bg-zinc-900 p-1 shadow-xl ${width}`}
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
  // selected marks the item that is the current choice, in a menu used as a
  // drop-down list.
  selected?: boolean;
  title?: string;
  children: ReactNode;
};

export function MenuItem({ onClick, disabled, danger, selected, title, children }: ItemProps) {
  return (
    <button
      disabled={disabled}
      title={title}
      onClick={onClick}
      className={`flex items-center gap-2 rounded px-3 py-2 text-left text-sm transition-colors disabled:pointer-events-none disabled:opacity-40 ${
        danger
          ? "text-red-400 hover:bg-red-500/10"
          : selected
            ? "bg-zinc-800 text-zinc-100"
            : "text-zinc-200 hover:bg-zinc-800"
      }`}
    >
      {children}
    </button>
  );
}

export function MenuDivider() {
  return <div className="my-1 border-t border-zinc-800" />;
}

// anchorBelow is where a menu opened from a button goes: under it, lined up
// with its left edge.
export function anchorBelow(el: HTMLElement): MenuAnchor {
  const r = el.getBoundingClientRect();
  return { x: r.left, y: r.bottom + 4 };
}
