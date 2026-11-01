import { useEffect, useRef, useState } from "react";
import { CreationService, errorMessage, LoadOrderEntry } from "../../api";
import { ErrorText, Spinner, Toggle } from "../ui";
import { useRefresh } from "../../refresh";
import { Grip } from "./ModListTab";
import { useProfile } from "./ProfileContext";
import { useDragReorder } from "./useDragReorder";

// PluginsPanel is the load order of a Creation Engine profile, as MO2's
// plugin list, in a panel beside the list of mods: the plugins the game will load with
// it, top to bottom, each with the index it gets in form ids. The game's own
// plugins come first and stay; the rest are dragged into order and switched
// on and off. The game loads masters before the other plugins whatever the
// order, so a plugin dragged among them goes back below them. A plugin
// missing a master is marked: the game does not start with it.
//
// columns lay out the header and every row alike: the grip, the index,
// Active and the name, with the kind of plugin after it.
const columns = "grid grid-cols-[0.75rem_3rem_2.75rem_minmax(0,1fr)] items-center gap-x-2";

export default function PluginsPanel() {
  const { game, profile } = useProfile();
  const [plugins, setPlugins] = useState<LoadOrderEntry[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  // Mods switched on or off bring plugins or take them away.
  const enabled = (profile.mods ?? [])
    .filter((m) => m.enabled && !m.separator)
    .map((m) => m.id)
    .join("\n");
  const [rescans, setRescans] = useState(0);
  useRefresh(() => setRescans((n) => n + 1));
  useEffect(() => {
    let current = true;
    CreationService.Plugins(game.id, profile.id)
      .then((list) => current && setPlugins(list ?? []))
      .catch((err) => current && setError(errorMessage(err)));
    return () => {
      current = false;
    };
  }, [game.id, profile.id, enabled, rescans]);

  const change = async (action: () => Promise<LoadOrderEntry[] | null>) => {
    setBusy(true);
    setError("");
    try {
      setPlugins((await action()) ?? []);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const scroller = useRef<HTMLDivElement>(null);
  const { drag, start } = useDragReorder(scroller, (ids, before) =>
    change(() => CreationService.MovePlugins(game.id, profile.id, ids, before ?? "")),
  );

  const modNames = new Map((profile.mods ?? []).map((m) => [m.id, m.name]));
  const origin = (p: LoadOrderEntry) => (p.mod === "" ? game.name : (modNames.get(p.mod) ?? "Overwrite"));
  const missing = (plugins ?? []).filter((p) => p.missing?.length).length;
  const active = (plugins ?? []).filter((p) => p.active).length;

  if (plugins === null) {
    return <div className="flex h-full items-center justify-center">{error ? <ErrorText>{error}</ErrorText> : <Spinner />}</div>;
  }

  return (
    <div ref={scroller} className="relative h-full overflow-y-auto px-3 pb-3">
      {/* The header stays in place while the list scrolls under it. */}
      <div className="sticky top-0 z-10 -mx-3 bg-zinc-950">
        <div className="flex items-baseline gap-2 px-3 pt-3 pb-1">
          <span className="text-sm font-semibold">Plugins</span>
          <span className="text-xs text-zinc-500">
            {active} of {plugins.length} on
          </span>
        </div>
        {error && (
          <div className="px-3">
            <ErrorText>{error}</ErrorText>
          </div>
        )}
        {missing > 0 && (
          <p className="mx-3 my-1 rounded-md border border-red-500/30 bg-red-500/10 px-2 py-1.5 text-xs text-red-300">
            {missing === 1 ? "A plugin is" : `${missing} plugins are`} missing masters: the game will not start until
            they load first or {missing === 1 ? "it is" : "these are"} switched off.
          </p>
        )}
        <div
          className={`${columns} border-b border-l-2 border-zinc-800 border-l-transparent px-3 py-1.5 text-xs font-medium text-zinc-500`}
        >
          <span />
          <span>Index</span>
          <span>On</span>
          <span>Name</span>
        </div>
      </div>
      <ul className="divide-y divide-zinc-800">
        {plugins.map((p, i) => (
          <li
            key={p.name}
            data-drag-row={p.name}
            onPointerDown={(e) => !p.implicit && !busy && start(e, [p.name], p.name)}
            title={`${p.name}\nfrom ${origin(p)}`}
            className={`${columns} -mx-3 border-l-2 px-3 py-1.5 ${drag?.ids.includes(p.name) ? "opacity-40" : ""} ${
              p.missing?.length || p.error ? "border-transparent bg-red-500/10" : `border-transparent ${drag ? "" : "hover:bg-zinc-800/60"}`
            }`}
          >
            {p.implicit ? <span /> : <Grip />}
            <span className="font-mono text-xs text-zinc-500 tabular-nums">{p.index}</span>
            <Toggle
              checked={p.active}
              disabled={p.implicit || busy}
              title={p.implicit ? "The game loads it by itself" : p.active ? "Switch off" : "Switch on"}
              onChange={(active) => change(() => CreationService.SetPluginActive(game.id, profile.id, p.name, active))}
            />
            <div className={`flex min-w-0 flex-col ${p.active ? "" : "opacity-50"}`}>
              <span className="flex min-w-0 items-center gap-1.5">
                <span className={`truncate text-sm ${p.implicit ? "text-zinc-400" : ""}`}>{p.name}</span>
                {p.master && <Kind title="Master: loaded before the plugins that are not">ESM</Kind>}
                {p.light && <Kind title="Light: shares the FE index with the other light plugins">ESL</Kind>}
              </span>
              {p.missing?.length ? (
                <span className="truncate text-xs text-red-300" title={p.missing.join("\n")}>
                  Missing {p.missing.length === 1 ? "master" : "masters"}: {p.missing.join(", ")}
                </span>
              ) : p.error ? (
                <span className="truncate text-xs text-red-300">{p.error}</span>
              ) : null}
            </div>
          </li>
        ))}
      </ul>
      {drag && (
        <>
          <div className="pointer-events-none absolute right-3 left-3 h-0.5 bg-accent-500" style={{ top: drag.line - 1 }} />
          <div
            className="pointer-events-none fixed z-50 max-w-xs truncate rounded-md border border-zinc-700 bg-zinc-800 px-3 py-1.5 text-sm shadow-xl"
            style={{ left: drag.x + 14, top: drag.y - 14 }}
          >
            {drag.label}
          </div>
        </>
      )}
    </div>
  );
}

function Kind({ title, children }: { title: string; children: string }) {
  return (
    <span title={title} className="shrink-0 rounded bg-zinc-800 px-1 py-px text-[10px] font-semibold text-zinc-400">
      {children}
    </span>
  );
}
