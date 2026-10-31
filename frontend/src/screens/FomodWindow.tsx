import { useEffect, useMemo, useState } from "react";
import { Window } from "@wailsio/runtime";
import { ModListService, errorMessage, FomodView, PreparedMod } from "../api";

// Choices are the options picked: per step, per group, the options' indices.
type Choices = number[][][];

// A view as the page uses it: Go's empty lists can arrive as null.
type Step = { index: number; name: string; groups: Group[] };
type Group = { name: string; type: string; problem: string; plugins: Option[] };
type Option = { name: string; description: string; image: string; type: string; selected: boolean };
type View = { steps: Step[]; choices: Choices; ready: boolean };

function normal(v: FomodView): View {
  return {
    ready: v.ready,
    choices: (v.choices ?? []).map((s) => (s ?? []).map((g) => g ?? [])),
    steps: (v.steps ?? []).map((s) => ({
      index: s.index,
      name: s.name,
      groups: (s.groups ?? []).map((g) => ({ ...g, plugins: g.plugins ?? [] })),
    })),
  };
}
import WindowFrame from "../components/WindowFrame";
import { Button, ErrorText, Spinner } from "../components/ui";

type Focus = { group: number; plugin: number };

const groupHints: Record<string, string> = {
  SelectExactlyOne: "Pick one",
  SelectAtMostOne: "Pick one or none",
  SelectAtLeastOne: "Pick at least one",
  SelectAll: "All of these are installed",
  SelectAny: "Pick any",
};

// FomodWindow is the window a mod's FOMOD installer runs in: its steps on
// the left, the options of the step in the middle and what the option under
// the pointer is about on the right. Every change of an option is worked out
// again by Hermit, so steps that depend on earlier options come and go as
// they are picked.
// position and total place the installer among those of a queue of installs.
export default function FomodWindow({ session, position, total }: { session: string; position: number; total: number }) {
  const [info, setInfo] = useState<PreparedMod | null>(null);
  const [view, setView] = useState<View | null>(null);
  const [at, setAt] = useState(0);
  const [focus, setFocus] = useState<Focus | null>(null);
  const [images, setImages] = useState<Record<string, string>>({});
  const [installing, setInstalling] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    Promise.all([ModListService.PreparedInfo(session), ModListService.FomodView(session, null)])
      .then(([i, v]) => {
        setInfo(i);
        setView(normal(v));
      })
      .catch((err) => setError(errorMessage(err)));
  }, [session]);

  const steps = view?.steps ?? [];
  const step = steps[Math.min(at, Math.max(steps.length - 1, 0))];
  const last = at >= steps.length - 1;
  const stepReady = !!step && step.groups.every((g) => !g.problem);

  // The option shown on the right: the one under the pointer, or the first
  // one picked in the step.
  const shown = useMemo(() => {
    if (!step) return null;
    if (focus) return step.groups[focus.group]?.plugins[focus.plugin] ?? null;
    for (const g of step.groups) {
      const p = g.plugins.find((p) => p.selected);
      if (p) return p;
    }
    return step.groups[0]?.plugins[0] ?? null;
  }, [step, focus]);
  const image = shown?.image || info?.image || "";

  useEffect(() => {
    if (!image || images[image] !== undefined) return;
    ModListService.FomodImage(session, image)
      .then((src) => setImages((m) => ({ ...m, [image]: src })))
      .catch(() => setImages((m) => ({ ...m, [image]: "" })));
  }, [image, images, session]);

  const toggle = async (g: number, i: number) => {
    if (!view || !step) return;
    const group = step.groups[g];
    const plugin = group.plugins[i];
    if (plugin.type === "NotUsable" || plugin.type === "Required" || group.type === "SelectAll") return;
    const current = view.choices[step.index]?.[g] ?? [];
    let next: number[];
    if (group.type === "SelectExactlyOne") next = [i];
    else if (group.type === "SelectAtMostOne") next = current.includes(i) ? [] : [i];
    else next = current.includes(i) ? current.filter((x) => x !== i) : [...current, i];
    const choices = view.choices.map((s, si) =>
      si === step.index ? (s ?? []).map((sel, gi) => (gi === g ? next : sel)) : s,
    );
    try {
      setView(normal(await ModListService.FomodView(session, choices)));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const cancel = () => {
    ModListService.CancelInstall(session).catch(() => {});
  };

  const install = async () => {
    if (!view) return;
    setInstalling(true);
    setError("");
    try {
      await ModListService.FinishInstall(session, view.choices);
      Window.Close();
    } catch (err) {
      setInstalling(false);
      setError(errorMessage(err));
    }
  };

  return (
    <WindowFrame title={info ? `Install ${info.name}` : "Install"} onClose={cancel}>
      <div className="flex h-full flex-col">
        <header className="flex items-baseline gap-3 border-b border-zinc-800 px-6 py-3">
          <span className="text-lg font-semibold">{info?.name ?? "…"}</span>
          <span className="text-sm text-zinc-500">
            {[info?.version, info?.author && `by ${info.author}`].filter(Boolean).join(" · ")}
          </span>
          {total > 1 && (
            <span
              className="ml-auto shrink-0 rounded-md bg-zinc-800 px-2 py-0.5 text-sm font-medium text-zinc-300 tabular-nums"
              title="Installers in the queue"
            >
              {position}/{total}
            </span>
          )}
        </header>

        <div className="flex min-h-0 flex-1">
          <nav className="w-52 shrink-0 overflow-y-auto border-r border-zinc-800 p-2">
            {steps.map((s, i) => (
              <button
                key={s.index}
                onClick={() => {
                  setAt(i);
                  setFocus(null);
                }}
                className={`flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm ${
                  i === at ? "bg-zinc-800 text-zinc-100" : "text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200"
                }`}
              >
                <span className="w-4 shrink-0 text-xs text-zinc-600 tabular-nums">{i + 1}</span>
                <span className="truncate">{s.name || "Options"}</span>
              </button>
            ))}
          </nav>

          <section className="min-w-0 flex-1 overflow-y-auto px-6 py-4">
            {!view && !error && <Spinner />}
            {step?.groups.map((group, g) => {
              const radio = group.type === "SelectExactlyOne" || group.type === "SelectAtMostOne";
              return (
                <div key={g} className="mb-5">
                  <div className="mb-2 flex items-baseline gap-2">
                    <span className="text-sm font-semibold">{group.name}</span>
                    <span className="text-xs text-zinc-500">{groupHints[group.type] ?? ""}</span>
                    {group.problem && <span className="text-xs text-amber-400">{group.problem}</span>}
                  </div>
                  <ul className="flex flex-col gap-0.5">
                    {group.plugins.map((p, i) => {
                      const locked = p.type === "NotUsable" || p.type === "Required" || group.type === "SelectAll";
                      return (
                        <li key={i}>
                          <button
                            disabled={p.type === "NotUsable"}
                            onMouseEnter={() => setFocus({ group: g, plugin: i })}
                            onClick={() => toggle(g, i)}
                            className={`flex w-full items-center gap-3 rounded-md px-3 py-2 text-left text-sm disabled:opacity-40 ${
                              p.selected ? "bg-accent-500/15 text-zinc-100" : "text-zinc-300 hover:bg-zinc-800/60"
                            } ${locked ? "cursor-default" : ""}`}
                          >
                            <span
                              className={`flex h-4 w-4 shrink-0 items-center justify-center border ${radio ? "rounded-full" : "rounded"} ${
                                p.selected ? "border-accent-400 bg-accent-500" : "border-zinc-600"
                              }`}
                            >
                              {p.selected && <span className={`h-1.5 w-1.5 bg-white ${radio ? "rounded-full" : "rounded-sm"}`} />}
                            </span>
                            <span className="min-w-0 flex-1 truncate">{p.name}</span>
                            {p.type !== "Optional" && (
                              <span
                                className={`shrink-0 text-xs ${
                                  p.type === "Recommended" ? "text-emerald-400" : p.type === "Required" ? "text-accent-300" : "text-zinc-500"
                                }`}
                              >
                                {p.type === "NotUsable" ? "Not usable" : p.type === "CouldBeUsable" ? "Could be usable" : p.type}
                              </span>
                            )}
                          </button>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              );
            })}
          </section>

          <aside className="flex w-80 shrink-0 flex-col gap-3 overflow-y-auto border-l border-zinc-800 p-4">
            {image && images[image] && (
              <img src={images[image]} alt="" className="max-h-60 w-full rounded-md object-contain" />
            )}
            {shown && <span className="text-sm font-semibold">{shown.name}</span>}
            <p className="text-sm whitespace-pre-wrap text-zinc-400 select-text">{shown?.description}</p>
          </aside>
        </div>

        <footer className="flex items-center gap-2 border-t border-zinc-800 px-6 py-3">
          <div className="min-w-0 flex-1">
            <ErrorText>{error}</ErrorText>
          </div>
          <Button
            variant="ghost"
            disabled={installing}
            onClick={() => {
              cancel();
              Window.Close();
            }}
          >
            Cancel
          </Button>
          <Button disabled={at === 0 || installing} onClick={() => setAt(at - 1)}>
            Back
          </Button>
          {last ? (
            <Button variant="primary" disabled={!view?.ready || installing} onClick={install}>
              {installing ? (
                <span className="flex items-center gap-2">
                  <Spinner /> Installing…
                </span>
              ) : (
                "Install"
              )}
            </Button>
          ) : (
            <Button
              variant="primary"
              disabled={!stepReady}
              onClick={() => {
                setAt(at + 1);
                setFocus(null);
              }}
            >
              Next
            </Button>
          )}
        </footer>
      </div>
    </WindowFrame>
  );
}
