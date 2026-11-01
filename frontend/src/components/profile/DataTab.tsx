import { useCallback, useEffect, useState } from "react";
import { errorMessage, ModListService, PreviewEntry } from "../../api";
import { ErrorText, Spinner } from "../ui";
import { useRefresh } from "../../refresh";
import { useProfile } from "./ProfileContext";

// DataTab shows the game folder as the profile's enabled mods will make it
// once the game starts, as MO2's Data tab does, without deploying anything:
// each file with the mod it comes from, and the mods whose file by that
// name loses to it. A Creation Engine game opens on its Data folder.
// Folders are read as they are opened: a profile can bring tens of
// thousands of files.
export default function DataTab() {
  const { game, profile } = useProfile();
  const [files, setFiles] = useState<number | null>(null);
  const [folders, setFolders] = useState<Map<string, PreviewEntry[]>>(new Map());
  const [open, setOpen] = useState<Set<string>>(new Set());
  const [error, setError] = useState("");

  const modNames = new Map((profile.mods ?? []).map((m) => [m.id, m.name]));
  const sourceName = (source: string) => (source === "" ? game.name : (modNames.get(source) ?? "Overwrite"));

  // What the mods make of the game folder changes with which are on and
  // their order, and with files changed by hand, shown on a refresh.
  const enabled = (profile.mods ?? [])
    .filter((m) => m.enabled && !m.separator)
    .map((m) => m.id)
    .join("\n");
  const [rescans, setRescans] = useState(0);
  useRefresh(() => setRescans((n) => n + 1));

  const read = useCallback(
    (path: string) => ModListService.PreviewFolder(game.id, profile.id, path).then((list) => list ?? []),
    [game.id, profile.id],
  );

  useEffect(() => {
    let current = true;
    setError("");
    (async () => {
      const summary = await ModListService.PreviewGame(game.id, profile.id);
      // The folders that were open stay open, read again; the first time
      // Data is.
      const keep = open.size ? [...open] : summary.open ? [summary.open] : [];
      const listed = await Promise.all(["", ...keep].map(async (p) => [p, await read(p).catch(() => null)] as const));
      if (!current) return;
      setFolders(new Map(listed.filter(([, list]) => list !== null) as [string, PreviewEntry[]][]));
      setOpen(new Set(keep.filter((p) => listed.some(([q, list]) => q === p && list !== null))));
      setFiles(summary.files);
    })().catch((err) => current && setError(errorMessage(err)));
    return () => {
      current = false;
    };
    // open is read, not followed: opening a folder reads it on its own.
  }, [game.id, profile.id, enabled, rescans, read]);

  const toggle = async (e: PreviewEntry) => {
    if (open.has(e.path)) {
      setOpen((s) => {
        const next = new Set(s);
        next.delete(e.path);
        return next;
      });
      return;
    }
    try {
      const list = folders.get(e.path) ?? (await read(e.path));
      setFolders((m) => new Map(m).set(e.path, list));
      setOpen((s) => new Set(s).add(e.path));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  // The rows of the open folders, depth first.
  const rows: { e: PreviewEntry; depth: number }[] = [];
  const add = (path: string, depth: number) => {
    for (const e of folders.get(path) ?? []) {
      rows.push({ e, depth });
      if (e.dir && open.has(e.path)) add(e.path, depth + 1);
    }
  };
  add("", 0);

  if (files === null) {
    return <div className="flex h-full items-center justify-center">{error ? <ErrorText>{error}</ErrorText> : <Spinner />}</div>;
  }

  return (
    <div className="h-full overflow-y-auto px-6 pb-3">
      <ErrorText>{error}</ErrorText>
      <div className="sticky top-0 z-10 -mx-6 grid grid-cols-[minmax(0,1fr)_minmax(0,22rem)] gap-x-3 border-b border-zinc-800 bg-zinc-950 px-6 py-2 text-xs font-medium text-zinc-500">
        <span>
          Name <span className="font-normal text-zinc-600">· {files.toLocaleString()} files</span>
        </span>
        <span>Mod</span>
      </div>
      <ul>
        {rows.map(({ e, depth }) => (
          <li
            key={e.path}
            onClick={e.dir ? () => toggle(e) : undefined}
            className={`-mx-6 grid grid-cols-[minmax(0,1fr)_minmax(0,22rem)] items-center gap-x-3 px-6 py-1 text-sm hover:bg-zinc-800/60 ${
              e.dir ? "cursor-pointer" : "cursor-default"
            }`}
          >
            <span className="flex min-w-0 items-center gap-1.5" style={{ paddingLeft: depth * 18 }}>
              {e.dir ? <Chevron open={open.has(e.path)} /> : <span className="w-3.5 shrink-0" />}
              {e.dir ? <FolderIcon /> : <FileIcon />}
              <span className={`truncate ${e.dir ? "text-zinc-200" : ""}`} title={e.path}>
                {e.name}
              </span>
              {e.dir && <span className="shrink-0 text-xs text-zinc-600">{e.files.toLocaleString()}</span>}
            </span>
            {e.dir ? (
              <span />
            ) : (
              <span className="flex min-w-0 items-baseline gap-2" title={sourceTitle(e, sourceName)}>
                <span className={`truncate ${e.source === "" ? "text-zinc-500" : "text-zinc-300"}`}>{sourceName(e.source)}</span>
                {(e.others ?? []).length > 0 && (
                  <span className="truncate text-xs text-zinc-600">over {(e.others ?? []).map(sourceName).join(", ")}</span>
                )}
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

function sourceTitle(e: PreviewEntry, name: (source: string) => string): string {
  if (!(e.others ?? []).length) return `From ${name(e.source)}`;
  return `From ${name(e.source)}, over the file of ${(e.others ?? []).map(name).join(", ")}`;
}

function Chevron({ open }: { open: boolean }) {
  return (
    <svg viewBox="0 0 24 24" className={`h-3.5 w-3.5 shrink-0 text-zinc-500 transition-transform ${open ? "rotate-90" : ""}`} fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="m9 6 6 6-6 6" />
    </svg>
  );
}

function FolderIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-4 w-4 shrink-0 text-accent-400" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2.5h8.5A1.5 1.5 0 0 1 21 9v9.5a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 18.5Z" />
    </svg>
  );
}

function FileIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-4 w-4 shrink-0 text-zinc-500" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z" />
      <path d="M14 3v5h5" />
    </svg>
  );
}
