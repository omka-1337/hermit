import { useEffect, useRef, useState } from "react";
import { Browser } from "@wailsio/runtime";
import { BethesdaService, confirmDanger, errorMessage, Mod } from "../../api";
import ContextMenu, { MenuAnchor, MenuItem } from "../ContextMenu";
import EditableName from "../EditableName";
import ImportMO2Modal from "../ImportMO2Modal";
import { Button, ErrorText, inputClass, Spinner, Toggle } from "../ui";
import { useProfile } from "./ProfileContext";
import { useDragReorder } from "./useDragReorder";

// BethesdaModsTab lists the mods of a Creation Engine profile in the order
// they are deployed into Data: where two mods have the same file, the one
// further down wins, as in Mod Organizer 2. Rows are dragged to change it; a
// folded separator takes its mods along. Separators head groups of the mods
// after them and fold them away. A right click on a row adds a separator next
// to it or removes the row.
// Clicking a mod shows its conflicts: in green the mods whose files it
// replaces, in red the ones replacing its files.
// columns lay out the header and every mod row alike: the grip, the place in
// the order, Active, Name, Version, Source and Author.
const columns = "grid grid-cols-[1rem_2rem_3.25rem_minmax(0,1fr)_6rem_7rem_10rem] items-center gap-x-3";

const sourceLabels: Record<string, string> = { nexus: "Nexus", local: "Local", github: "GitHub", thunderstore: "Thunderstore" };

export default function BethesdaModsTab() {
  const { game, profile, applyProfile, setEnabled } = useProfile();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  // naming is the separator being added: where it goes and its name so far.
  const [naming, setNaming] = useState<{ at: number; name: string } | null>(null);
  // mod is the row the menu is for; above and below are where a separator
  // added from it goes.
  const [menu, setMenu] = useState<{ mod: Mod; above: number; below: number; at: MenuAnchor } | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  // conflicts maps each mod to the mods it shares files with, and how many.
  const [conflicts, setConflicts] = useState<Record<string, Record<string, number>>>({});
  const mods = profile.mods ?? [];

  // Which mods share files changes only when mods come or go, not when they
  // are moved or switched on and off.
  const modSet = mods
    .filter((m) => !m.separator)
    .map((m) => m.id)
    .sort()
    .join("\n");
  useEffect(() => {
    let current = true;
    BethesdaService.Conflicts(game.id, profile.id)
      .then((found) => current && setConflicts((found ?? {}) as Record<string, Record<string, number>>))
      .catch(console.error);
    return () => {
      current = false;
    };
  }, [game.id, profile.id, modSet]);

  // relation is how a mod stands to the selected one: "replaced" when the
  // selected mod wins over some of its files, "replaces" when it wins over
  // the selected mod's. Mods that are off take no part.
  const selectedAt = mods.findIndex((m) => m.id === selected);
  const relation = (m: Mod, i: number): "replaced" | "replaces" | null => {
    const sel = mods[selectedAt];
    if (!sel || m.id === sel.id || !sel.enabled || !m.enabled || !conflicts[sel.id]?.[m.id]) return null;
    return i > selectedAt ? "replaces" : "replaced";
  };

  const scroller = useRef<HTMLDivElement>(null);
  const { drag, start, clickWasDrag } = useDragReorder(scroller, (ids, before) => reorder(ids, before));

  const select = (m: Mod) => (e: React.MouseEvent) => {
    if (clickWasDrag() || (e.target as HTMLElement).closest("button, input, a, select")) return;
    setSelected((s) => (s === m.id ? null : m.id));
  };

  const act = async (id: string, action: () => Promise<void>) => {
    setError("");
    setBusy(id);
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(null);
    }
  };

  // reorder shows the new order at once and then saves it, going back to the
  // old one if saving fails.
  const reorder = (ids: string[], before: string | null) => {
    const picked = mods.filter((m) => ids.includes(m.id));
    const rest = mods.filter((m) => !ids.includes(m.id));
    const at = before === null ? rest.length : rest.findIndex((m) => m.id === before);
    if (at < 0) return;
    const next = [...rest.slice(0, at), ...picked, ...rest.slice(at)];
    if (next.every((m, i) => m.id === mods[i].id)) return;
    const old = profile;
    applyProfile({ ...profile, mods: next });
    BethesdaService.MoveMods(game.id, profile.id, ids, before ?? "")
      .then(applyProfile)
      .catch((err) => {
        applyProfile(old);
        setError(errorMessage(err));
      });
  };

  // The rows a drag starting on a row carries: a folded separator brings the
  // mods under it.
  const dragRows = (m: Mod, i: number, count: number) =>
    m.separator && collapsed.has(m.id) ? mods.slice(i, i + 1 + count).map((n) => n.id) : [m.id];

  const remove = (m: Mod) =>
    act(m.id, async () => {
      // A separator has no files, and its mods stay where they are.
      if (m.separator || (await confirmDanger("Remove mod", `Remove ${m.name} and its files from this profile?`, "Remove"))) {
        applyProfile(await BethesdaService.RemoveMod(game.id, profile.id, m.id));
      }
    });

  const addSeparator = (name: string, at: number) =>
    act("separator", async () => {
      setNaming(null);
      if (name.trim()) applyProfile(await BethesdaService.AddSeparator(game.id, profile.id, name, at));
    });

  // Below a folded separator is past the mods it hides.
  const openMenu = (index: number, below: number) => (e: React.MouseEvent) => {
    e.preventDefault();
    setMenu({ mod: mods[index], above: index, below, at: { x: e.clientX, y: e.clientY } });
  };

  const nameInput = naming && (
    <li key="new-separator" className="py-1.5">
      <input
        autoFocus
        placeholder="Separator name"
        value={naming.name}
        className={inputClass}
        onChange={(e) => setNaming({ ...naming, name: e.target.value })}
        onBlur={() => addSeparator(naming.name, naming.at)}
        onKeyDown={(e) => {
          if (e.key === "Enter") e.currentTarget.blur();
          if (e.key === "Escape") setNaming(null);
        }}
      />
    </li>
  );

  const rename = (m: Mod, name: string) =>
    act(m.id, async () => applyProfile(await BethesdaService.RenameSeparator(game.id, profile.id, m.id, name)));

  const toggle = (id: string) =>
    setCollapsed((c) => {
      const next = new Set(c);
      if (!next.delete(id)) next.add(id);
      return next;
    });

  // Each row with what it needs to be drawn: a mod's place among the mods
  // alone, a separator's count of mods, and whether a folded separator hides it.
  let priority = 0;
  let hiddenBy: string | null = null;
  const rows = mods.map((m, i) => {
    if (m.separator) {
      hiddenBy = collapsed.has(m.id) ? m.id : null;
      const end = mods.findIndex((n, j) => j > i && n.separator);
      const group = mods.slice(i + 1, end < 0 ? undefined : end);
      // A folded group still shows that it holds mods in conflict.
      const relations = new Set(group.map((n, k) => relation(n, i + 1 + k)));
      return { m, i, count: group.length, priority: 0, hidden: false, rel: null, relations };
    }
    return { m, i, count: 0, priority: ++priority, hidden: hiddenBy !== null, rel: relation(m, i), relations: null };
  });
  const selectedName = mods[selectedAt]?.name;

  const importModal = importing && (
    <ImportMO2Modal
      game={game}
      profileId={profile.id}
      onClose={() => setImporting(false)}
      onImported={(imported) => {
        setImporting(false);
        applyProfile(imported);
      }}
    />
  );

  if (mods.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3">
        <p className="text-zinc-400">No mods in this profile yet.</p>
        <Button variant="primary" onClick={() => setImporting(true)}>
          Import from Mod Organizer 2…
        </Button>
        <p className="max-w-md text-center text-xs text-zinc-500">
          Brings over the mods of an MO2 profile as they are installed there, with their order, their separators, the
          options picked in their installers, the plugin list and the profile's INI files. MO2 itself is left as it is.
        </p>
        {importModal}
      </div>
    );
  }

  return (
    <div ref={scroller} className="relative h-full overflow-y-auto px-6 pb-3">
      {importModal}
      <ErrorText>{error}</ErrorText>
      {/* The header stays in place while the list scrolls under it. */}
      <div
        className={`${columns} sticky top-0 z-10 -mx-6 border-b border-l-2 border-zinc-800 border-l-transparent bg-zinc-950 px-6 py-2 text-xs font-medium text-zinc-500`}
      >
        <span />
        <span className="text-right">#</span>
        <span>Active</span>
        <span>Name</span>
        <span>Version</span>
        <span>Source</span>
        <span>Author</span>
      </div>
      <ul className="divide-y divide-zinc-800">
        {rows.map(({ m, i, count, priority, hidden, rel, relations }) => [
          naming?.at === i && nameInput,
          hidden ? null : m.separator ? (
            <li
              key={m.id}
              data-drag-row={m.id}
              onPointerDown={(e) => busy === null && start(e, dragRows(m, i, count), m.name)}
              onContextMenu={openMenu(i, collapsed.has(m.id) ? i + 1 + count : i + 1)}
              className={`-mx-6 flex items-center gap-2 border-l-2 border-transparent bg-zinc-900/60 px-6 py-1.5 ${
                drag?.ids.includes(m.id) ? "opacity-40" : ""
              }`}
            >
              <Grip />
              <button
                aria-label={collapsed.has(m.id) ? `Show the mods under ${m.name}` : `Fold the mods under ${m.name}`}
                onClick={() => toggle(m.id)}
                className="rounded p-1 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100"
              >
                <ChevronIcon open={!collapsed.has(m.id)} />
              </button>
              <EditableName value={m.name} className="text-sm font-bold text-zinc-100" onSave={(name) => rename(m, name)} />
              <span className="text-xs text-zinc-600">{count}</span>
              {collapsed.has(m.id) && relations?.has("replaced") && (
                <span className="h-2 w-2 rounded-full bg-emerald-500" title={`${selectedName} replaces files of mods here`} />
              )}
              {collapsed.has(m.id) && relations?.has("replaces") && (
                <span className="h-2 w-2 rounded-full bg-red-500" title={`Mods here replace files of ${selectedName}`} />
              )}
              <span className="flex-1" />
              {busy === m.id && <Spinner />}
            </li>
          ) : (
            <li
              key={m.id}
              data-drag-row={m.id}
              onPointerDown={(e) => busy === null && start(e, [m.id], m.name)}
              onContextMenu={openMenu(i, i + 1)}
              onClick={select(m)}
              title={
                rel === "replaced"
                  ? `${selectedName} replaces ${conflicts[selected!]?.[m.id]} of its files`
                  : rel === "replaces"
                    ? `Replaces ${conflicts[selected!]?.[m.id]} files of ${selectedName}`
                    : undefined
              }
              className={`${columns} cursor-default border-l-2 py-2 ${drag?.ids.includes(m.id) ? "opacity-40" : ""} ${
                m.id === selected
                  ? "border-indigo-500 bg-indigo-500/15"
                  : rel === "replaced"
                    ? "border-emerald-500 bg-emerald-500/10"
                    : rel === "replaces"
                      ? "border-red-500 bg-red-500/10"
                      : `border-transparent ${drag ? "" : "hover:bg-zinc-800/60"}`
              }`}
            >
              <Grip />
              <span className="text-right text-xs text-zinc-600 tabular-nums">{priority}</span>
              <Toggle
                first={i === 0}
                checked={m.enabled}
                disabled={busy !== null}
                title={m.enabled ? "Disable" : "Enable"}
                onChange={(enabled) => act(m.id, () => setEnabled(m.id, enabled))}
              />
              <div className={`flex min-w-0 flex-col ${m.enabled ? "" : "opacity-50"}`}>
                <span className="flex min-w-0 items-center gap-2 text-sm font-medium">
                  <span className="truncate">{m.name}</span>
                  {busy === m.id && <Spinner />}
                </span>
                {m.notes && (
                  <span className="truncate text-xs text-zinc-500" title={m.notes}>
                    {m.notes}
                  </span>
                )}
              </div>
              <span className={`truncate text-sm text-zinc-400 ${m.enabled ? "" : "opacity-50"}`} title={m.version}>
                {m.version}
              </span>
              <span className={`truncate text-sm ${m.enabled ? "" : "opacity-50"}`}>
                {m.source.type === "nexus" ? (
                  <button
                    title={m.source.url}
                    onClick={() => Browser.OpenURL(m.source.url!)}
                    className="text-indigo-400 hover:text-indigo-300 hover:underline"
                  >
                    Nexus
                  </button>
                ) : (
                  <span className="text-zinc-400">{sourceLabels[m.source.type] ?? ""}</span>
                )}
              </span>
              <span className={`truncate text-sm text-zinc-400 ${m.enabled ? "" : "opacity-50"}`}>{m.author}</span>
            </li>
          ),
        ])}
        {naming?.at === mods.length && nameInput}
      </ul>
      {drag && (
        <>
          <div className="pointer-events-none absolute right-6 left-6 h-0.5 bg-indigo-500" style={{ top: drag.line - 1 }} />
          <div
            className="pointer-events-none fixed z-50 max-w-xs truncate rounded-md border border-zinc-700 bg-zinc-800 px-3 py-1.5 text-sm shadow-xl"
            style={{ left: drag.x + 14, top: drag.y - 14 }}
          >
            {drag.label}
            {drag.ids.length > 1 && <span className="ml-2 text-zinc-400">+{drag.ids.length - 1}</span>}
          </div>
        </>
      )}
      {menu && (
        <ContextMenu at={menu.at} onClose={() => setMenu(null)}>
          <MenuItem
            onClick={() => {
              setNaming({ at: menu.above, name: "" });
              setMenu(null);
            }}
          >
            Add separator above
          </MenuItem>
          <MenuItem
            onClick={() => {
              setNaming({ at: menu.below, name: "" });
              setMenu(null);
            }}
          >
            Add separator below
          </MenuItem>
          <MenuItem
            danger
            disabled={busy !== null}
            onClick={() => {
              remove(menu.mod);
              setMenu(null);
            }}
          >
            {menu.mod.separator ? "Remove separator" : "Remove"}
          </MenuItem>
        </ContextMenu>
      )}
    </div>
  );
}

// Grip is where a finger drags a row from; a mouse can take the whole row.
function Grip() {
  return (
    <span data-drag-grip className="flex cursor-grab touch-none items-center self-stretch px-0.5 text-zinc-600" aria-hidden>
      <svg viewBox="0 0 10 16" className="h-4 w-2.5" fill="currentColor">
        <circle cx="2.5" cy="3" r="1.3" />
        <circle cx="7.5" cy="3" r="1.3" />
        <circle cx="2.5" cy="8" r="1.3" />
        <circle cx="7.5" cy="8" r="1.3" />
        <circle cx="2.5" cy="13" r="1.3" />
        <circle cx="7.5" cy="13" r="1.3" />
      </svg>
    </span>
  );
}

function ChevronIcon({ open }: { open: boolean }) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={`h-4 w-4 transition-transform ${open ? "rotate-90" : ""}`}
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="m9 18 6-6-6-6" />
    </svg>
  );
}
