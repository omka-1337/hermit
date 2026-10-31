import { useCallback, useEffect, useRef, useState, type RefObject } from "react";

// How far the pointer moves before a press becomes a drag, so a click stays
// a click.
const threshold = 4;
// Near the top or bottom edge of the list, within zone pixels, it scrolls by
// itself: slowly at the zone's border and faster the closer the pointer gets
// to the edge, up to maxSpeed pixels a frame at the edge and past it.
const zone = 80;
const maxSpeed = 30;

export type DragState = {
  // ids are the rows being moved, label what the pointer carries.
  ids: string[];
  label: string;
  x: number;
  y: number;
  // before is the row they go in front of, null for the end of the list.
  before: string | null;
  // line is where the drop line is drawn, from the top of the list's content.
  line: number;
};

type Pending = { ids: string[]; label: string; startX: number; startY: number; pointerId: number; target: HTMLElement };

// useDragReorder lets the rows of a scrolling list be dragged to a new
// place. Rows are marked with data-drag-row set to their id; onDrop gets the
// ids moved and the row they now go before.
export function useDragReorder(scroller: RefObject<HTMLElement | null>, onDrop: (ids: string[], before: string | null) => void) {
  const [drag, setDragState] = useState<DragState | null>(null);
  // current is the drag as the event handlers see it, between renders.
  const current = useRef<DragState | null>(null);
  const setDrag = useCallback((d: DragState | null) => {
    current.current = d;
    setDragState(d);
  }, []);
  const pending = useRef<Pending | null>(null);
  const pointer = useRef({ x: 0, y: 0 });
  const dragged = useRef(false);
  const onDropRef = useRef(onDrop);
  onDropRef.current = onDrop;

  // target finds where rows dropped at the pointer would go: in front of the
  // first row, not one of those moved, whose middle is below the pointer.
  const target = useCallback(
    (ids: string[]): { before: string | null; line: number } => {
      const box = scroller.current;
      if (!box) return { before: null, line: 0 };
      const top = box.getBoundingClientRect().top - box.scrollTop;
      let last: DOMRect | null = null;
      for (const row of box.querySelectorAll<HTMLElement>("[data-drag-row]")) {
        const id = row.dataset.dragRow!;
        if (ids.includes(id)) continue;
        const r = row.getBoundingClientRect();
        if (r.top + r.height / 2 > pointer.current.y) return { before: id, line: r.top - top };
        last = r;
      }
      return { before: null, line: last ? last.bottom - top : 0 };
    },
    [scroller],
  );

  const update = useCallback(() => {
    const d = current.current;
    if (d) setDrag({ ...d, x: pointer.current.x, y: pointer.current.y, ...target(d.ids) });
  }, [target, setDrag]);

  // Scrolls while the pointer is near an edge, for as long as the drag lasts.
  const dragging = drag !== null;
  useEffect(() => {
    if (!dragging) return;
    let frame = 0;
    const step = () => {
      const box = scroller.current;
      if (box) {
        const r = box.getBoundingClientRect();
        const y = pointer.current.y;
        const into = y < r.top + zone ? r.top + zone - y : y > r.bottom - zone ? -(y - (r.bottom - zone)) : 0;
        if (into !== 0) {
          const closeness = Math.min(1, Math.abs(into) / zone);
          const before = box.scrollTop;
          box.scrollTop += (into > 0 ? -1 : 1) * Math.max(1, maxSpeed * closeness * closeness);
          if (box.scrollTop !== before) update();
        }
      }
      frame = requestAnimationFrame(step);
    };
    frame = requestAnimationFrame(step);
    return () => cancelAnimationFrame(frame);
  }, [dragging, scroller, update]);

  useEffect(() => {
    const move = (e: PointerEvent) => {
      pointer.current = { x: e.clientX, y: e.clientY };
      const p = pending.current;
      if (p && !dragged.current) {
        if (Math.hypot(e.clientX - p.startX, e.clientY - p.startY) < threshold) return;
        dragged.current = true;
        // Keep the pointer's events coming when it leaves the window.
        p.target.setPointerCapture(p.pointerId);
        document.body.classList.add("dragging");
        setDrag({ ids: p.ids, label: p.label, x: e.clientX, y: e.clientY, ...target(p.ids) });
        return;
      }
      if (dragged.current) update();
    };
    const finish = (drop: boolean) => {
      const d = current.current;
      pending.current = null;
      document.body.classList.remove("dragging");
      setDrag(null);
      if (d && drop) onDropRef.current(d.ids, d.before);
    };
    const up = () => finish(true);
    const cancel = () => finish(false);
    const key = (e: KeyboardEvent) => e.key === "Escape" && dragged.current && finish(false);
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", cancel);
    window.addEventListener("keydown", key);
    return () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", cancel);
      window.removeEventListener("keydown", key);
    };
  }, [target, update, setDrag]);

  // start is called from a row's pointerdown. A mouse drags a row from
  // anywhere but its buttons; a finger only from the grip, so that swiping
  // the list still scrolls it.
  const start = (e: React.PointerEvent, ids: string[], label: string) => {
    const el = e.target as HTMLElement;
    const onGrip = !!el.closest("[data-drag-grip]");
    if (e.button !== 0 || (!onGrip && (e.pointerType !== "mouse" || el.closest("button, input, a, select")))) return;
    dragged.current = false;
    pointer.current = { x: e.clientX, y: e.clientY };
    pending.current = { ids, label, startX: e.clientX, startY: e.clientY, pointerId: e.pointerId, target: e.currentTarget as HTMLElement };
  };

  // clickWasDrag tells a row's click handler that the click ends a drag and
  // is not a click.
  const clickWasDrag = () => {
    const was = dragged.current;
    dragged.current = false;
    return was;
  };

  return { drag, start, clickWasDrag };
}
