import { useEffect, useRef, useState } from "react";
import { SettingsStore } from "./api";

// The widths panels were dragged to, read from the settings once and shared
// by every panel.
let saved: Promise<Record<string, number | undefined>> | null = null;
function savedWidths() {
  saved ??= Promise.resolve(SettingsStore.Get())
    .then((s): Record<string, number | undefined> => s.panelWidths ?? {})
    .catch(() => ({}));
  return saved;
}

// usePanelWidth is the width of a panel the user can resize: what it was
// dragged to last time, kept between min and max, or fallback until it is.
export function usePanelWidth(name: string, fallback: number, min: number, max: number) {
  const clamp = (w: number) => Math.round(Math.min(max, Math.max(min, w)));
  const [width, setWidth] = useState(fallback);
  useEffect(() => {
    let current = true;
    savedWidths().then((widths) => {
      const w = widths[name];
      if (current && w) setWidth(clamp(w));
    });
    return () => {
      current = false;
    };
    // The limits are constants of the panel, so only the name matters.
  }, [name]);
  const commit = (w: number) => {
    const at = clamp(w);
    saved = savedWidths().then((widths) => ({ ...widths, [name]: at }));
    SettingsStore.SetPanelWidth(name, at).catch(console.error);
  };
  return {
    width,
    resize: (w: number) => setWidth(clamp(w)),
    commit,
    reset: () => {
      setWidth(fallback);
      commit(fallback);
    },
  };
}

type HandleProps = {
  // edge is the side of the panel the handle is on: dragging it away from
  // the panel widens the panel.
  edge: "left" | "right";
  panel: ReturnType<typeof usePanelWidth>;
  label: string;
};

// ResizeHandle is the border of a panel, dragged to change the panel's
// width; a double click gives it back its first width. The panel must be
// positioned for it to sit on its edge.
export function ResizeHandle({ edge, panel, label }: HandleProps) {
  const drag = useRef<{ x: number; width: number; last: number } | null>(null);
  const [active, setActive] = useState(false);
  const widthAt = (x: number) => {
    const d = drag.current!;
    return d.width + (edge === "right" ? x - d.x : d.x - x);
  };
  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      title={`${label}: drag to resize, double-click to reset`}
      onPointerDown={(e) => {
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        drag.current = { x: e.clientX, width: panel.width, last: panel.width };
        setActive(true);
      }}
      onPointerMove={(e) => {
        if (!drag.current) return;
        drag.current.last = widthAt(e.clientX);
        panel.resize(drag.current.last);
      }}
      onPointerUp={() => {
        if (!drag.current) return;
        panel.commit(drag.current.last);
        drag.current = null;
        setActive(false);
      }}
      onPointerCancel={() => {
        drag.current = null;
        setActive(false);
      }}
      onDoubleClick={panel.reset}
      className={`group absolute inset-y-0 z-20 w-2 cursor-col-resize touch-none ${edge === "right" ? "-right-1" : "-left-1"}`}
    >
      <div
        className={`mx-auto h-full w-0.5 transition-colors ${active ? "bg-accent-500" : "bg-transparent group-hover:bg-accent-500/60"}`}
      />
    </div>
  );
}
