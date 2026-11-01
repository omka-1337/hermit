import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "./components/ui";
import { Question, setConfirmHost } from "./confirmhost";
import { clickFocused, focusFirst, moveFocus } from "./focus";
import { useGamepad } from "./gamepad";
import { useModalLayer } from "./modals";
import { useLayout } from "./uimode";

type Pending = { question: Question; answer: (ok: boolean) => void };

// ConfirmHost draws questions inside the window on the deck, where a system
// dialog is another window the controller cannot reach.
export default function ConfirmHost() {
  const deck = useLayout() === "deck";
  const [pending, setPending] = useState<Pending | null>(null);
  const box = useRef<HTMLDivElement>(null);
  const top = useModalLayer(pending !== null);

  const request = useCallback(
    (question: Question) => new Promise<boolean>((answer) => setPending({ question, answer })),
    [],
  );

  useEffect(() => {
    if (!deck) return;
    setConfirmHost(request);
    return () => setConfirmHost(null);
  }, [deck, request]);

  const close = (ok: boolean) => {
    pending?.answer(ok);
    setPending(null);
  };

  useEffect(() => {
    if (pending && top) focusFirst(box.current);
  }, [pending, top]);

  useGamepad(
    {
      onLeft: () => moveFocus("left", box.current),
      onRight: () => moveFocus("right", box.current),
      onUp: () => moveFocus("up", box.current),
      onDown: () => moveFocus("down", box.current),
      onAccept: () => clickFocused(box.current),
      onBack: () => close(false),
    },
    pending !== null && top,
  );

  if (!pending) return null;
  const { question } = pending;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-6" onClick={() => close(false)}>
      <div
        ref={box}
        className="flex w-full max-w-lg flex-col gap-4 rounded-xl border border-zinc-700 bg-zinc-900 p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <span className="text-lg font-semibold">{question.title}</span>
        <p className="text-sm whitespace-pre-line text-zinc-300">{question.message}</p>
        <div className="flex justify-end gap-2">
          <Button
            variant="ghost"
            data-focus-first={question.danger || undefined}
            onClick={() => close(false)}
          >
            Cancel (B)
          </Button>
          <Button
            variant={question.danger ? "danger" : "primary"}
            data-focus-first={question.danger ? undefined : true}
            onClick={() => close(true)}
          >
            {question.action} (A)
          </Button>
        </div>
      </div>
    </div>
  );
}
