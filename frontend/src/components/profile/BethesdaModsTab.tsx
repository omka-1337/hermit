import { useState } from "react";
import { Browser, Dialogs } from "@wailsio/runtime";
import { BethesdaService, confirmDanger, errorMessage, MO2Instance, Mod } from "../../api";
import ContextMenu, { MenuAnchor, MenuItem } from "../ContextMenu";
import EditableName from "../EditableName";
import { Button, ErrorText, inputClass, Modal, Spinner, Toggle } from "../ui";
import { useProfile } from "./ProfileContext";

// BethesdaModsTab lists the mods of a Creation Engine profile in the order
// they are deployed into Data: where two mods have the same file, the one
// further down wins, as in Mod Organizer 2. Separators head groups of the
// mods after them and fold them away; a right click on a row adds one next
// to it.
export default function BethesdaModsTab() {
  const { game, profile, applyProfile, setEnabled } = useProfile();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  // naming is the separator being added: where it goes and its name so far.
  const [naming, setNaming] = useState<{ at: number; name: string } | null>(null);
  // above and below are where a separator added from the menu goes.
  const [menu, setMenu] = useState<{ above: number; below: number; at: MenuAnchor } | null>(null);
  const mods = profile.mods ?? [];

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

  const move = (m: Mod, to: number) =>
    act(m.id, async () => applyProfile(await BethesdaService.MoveMod(game.id, profile.id, m.id, to)));

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
    setMenu({ above: index, below, at: { x: e.clientX, y: e.clientY } });
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
      const count = mods.slice(i + 1, end < 0 ? undefined : end).length;
      return { m, i, count, priority: 0, hidden: false };
    }
    return { m, i, count: 0, priority: ++priority, hidden: hiddenBy !== null };
  });

  const importModal = importing && <ImportMO2Modal onClose={() => setImporting(false)} />;

  if (mods.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3">
        <p className="text-zinc-400">No mods in this profile yet.</p>
        <Button variant="primary" onClick={() => setImporting(true)}>
          Import from Mod Organizer 2…
        </Button>
        <p className="max-w-md text-center text-xs text-zinc-500">
          Brings over the mods of an MO2 profile as they are installed there, with their order, their separators, the
          options picked in their installers and the plugin list. MO2 itself is left as it is.
        </p>
        {importModal}
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto px-6 py-3">
      {importModal}
      <ErrorText>{error}</ErrorText>
      <ul className="divide-y divide-zinc-800">
        {rows.map(({ m, i, count, priority, hidden }) => [
          naming?.at === i && nameInput,
          hidden ? null : m.separator ? (
            <li
              key={m.id}
              onContextMenu={openMenu(i, collapsed.has(m.id) ? i + 1 + count : i + 1)}
              className="flex items-center gap-2 bg-zinc-900/60 py-1.5 pr-0 pl-1"
            >
              <button
                aria-label={collapsed.has(m.id) ? `Show the mods under ${m.name}` : `Fold the mods under ${m.name}`}
                onClick={() => toggle(m.id)}
                className="rounded p-1 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100"
              >
                <ChevronIcon open={!collapsed.has(m.id)} />
              </button>
              <EditableName
                value={m.name}
                className="text-xs font-semibold tracking-wide text-zinc-300 uppercase"
                onSave={(name) => rename(m, name)}
              />
              <span className="text-xs text-zinc-600">{count}</span>
              <span className="flex-1" />
              {busy === m.id && <Spinner />}
              <MoveButtons i={i} last={mods.length - 1} busy={busy !== null} onMove={(to) => move(m, to)} />
              <Button variant="danger" disabled={busy !== null} onClick={() => remove(m)}>
                Remove
              </Button>
            </li>
          ) : (
            <li key={m.id} onContextMenu={openMenu(i, i + 1)} className="flex items-center gap-3 py-2">
              <span className="w-7 shrink-0 text-right text-xs text-zinc-600 tabular-nums">{priority}</span>
              <Toggle
                first={i === 0}
                checked={m.enabled}
                disabled={busy !== null}
                title={m.enabled ? "Disable" : "Enable"}
                onChange={(enabled) => act(m.id, () => setEnabled(m.id, enabled))}
              />
              <div className={`flex min-w-0 flex-1 flex-col ${m.enabled ? "" : "opacity-50"}`}>
                <span className="truncate text-sm font-medium">
                  {m.name}
                  {m.version && <span className="ml-2 font-normal text-zinc-500">{m.version}</span>}
                </span>
                {m.notes && (
                  <span className="truncate text-xs text-zinc-500" title={m.notes}>
                    {m.notes}
                  </span>
                )}
              </div>
              {busy === m.id && <Spinner />}
              {m.source.type === "nexus" && (
                <Button variant="ghost" title={m.source.url} onClick={() => Browser.OpenURL(m.source.url!)}>
                  Nexus
                </Button>
              )}
              <MoveButtons i={i} last={mods.length - 1} busy={busy !== null} onMove={(to) => move(m, to)} />
              <Button variant="danger" disabled={busy !== null} onClick={() => remove(m)}>
                Remove
              </Button>
            </li>
          ),
        ])}
        {naming?.at === mods.length && nameInput}
      </ul>
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
        </ContextMenu>
      )}
    </div>
  );
}

function MoveButtons({ i, last, busy, onMove }: { i: number; last: number; busy: boolean; onMove: (to: number) => void }) {
  return (
    <>
      <Button variant="ghost" title="Earlier: lose to more mods" disabled={busy || i === 0} onClick={() => onMove(i - 1)}>
        ↑
      </Button>
      <Button variant="ghost" title="Later: win over more mods" disabled={busy || i === last} onClick={() => onMove(i + 1)}>
        ↓
      </Button>
    </>
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

function ImportMO2Modal({ onClose }: { onClose: () => void }) {
  const { game, profile, applyProfile } = useProfile();
  const [instance, setInstance] = useState<MO2Instance | null>(null);
  const [chosen, setChosen] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const pick = async () => {
    setError("");
    try {
      const dir = await Dialogs.OpenFile({
        Title: "Choose the Mod Organizer 2 folder, the one with ModOrganizer.ini",
        CanChooseDirectories: true,
        CanChooseFiles: false,
      });
      if (!dir) return;
      const found = await BethesdaService.InspectMO2(dir);
      setInstance(found);
      setChosen(found.profiles?.includes(found.selected) ? found.selected : (found.profiles?.[0] ?? ""));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const run = async () => {
    if (!instance) return;
    setBusy(true);
    setError("");
    try {
      applyProfile(await BethesdaService.ImportMO2(game.id, profile.id, instance.dir, chosen));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title="Import from Mod Organizer 2" onClose={busy ? () => {} : onClose}>
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-zinc-400">MO2 folder</span>
          <div className="flex gap-2">
            <div className="flex-1 truncate rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-300 select-text">
              {instance?.dir ?? <span className="text-zinc-500">Not selected</span>}
            </div>
            <Button onClick={pick} disabled={busy}>
              Browse…
            </Button>
          </div>
          {instance && <span className="text-xs text-zinc-500">Set up for {instance.gameName}</span>}
        </div>
        {instance && (instance.profiles?.length ?? 0) > 1 && (
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-zinc-400">MO2 profile</span>
            <select className={inputClass} value={chosen} disabled={busy} onChange={(e) => setChosen(e.target.value)}>
              {instance.profiles!.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </label>
        )}
        <p className="text-xs text-zinc-500">
          On the disk MO2 is on, files are linked rather than copied: the import takes seconds and no space. Move this
          profile to that disk first (right click on it, Move to folder…) if it is not there yet.
        </p>
        <ErrorText>{error}</ErrorText>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" onClick={run} disabled={!instance || !chosen || busy}>
            {busy ? (
              <span className="flex items-center gap-2">
                <Spinner /> Importing…
              </span>
            ) : (
              "Import"
            )}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
