import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Change,
  ConfigContent,
  ConfigFileInfo,
  ConfigService,
  confirmDanger,
  Entry,
  errorMessage,
  LaunchService,
} from "../../api";
import { Button, ErrorText, inputClass, Toggle } from "../ui";
import { useProfile } from "./ProfileContext";

export default function ConfigTab() {
  const { game, profile } = useProfile();
  const [files, setFiles] = useState<ConfigFileInfo[]>([]);
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const [error, setError] = useState("");

  const loadFiles = useCallback(() => {
    ConfigService.ListConfigs(game.id, profile.id)
      .then((list) => setFiles(sortFiles(list ?? [])))
      .catch((err) => setError(errorMessage(err)));
  }, [game.id, profile.id]);

  useEffect(loadFiles, [loadFiles]);

  const select = async (path: string) => {
    if (path === selected) return;
    if (dirty && !(await confirmDanger("Unsaved changes", "Discard unsaved changes to this file?", "Discard"))) {
      return;
    }
    setDirty(false);
    setSelected(path);
  };

  const shown = files.filter((f) => `${f.plugin} ${f.path}`.toLowerCase().includes(filter.trim().toLowerCase()));

  return (
    <div className="flex h-full min-h-0">
      <aside className="flex w-80 shrink-0 flex-col border-r border-zinc-800">
        <div className="flex gap-2 p-3">
          <input
            className={inputClass}
            placeholder="Filter config files"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
          <Button variant="ghost" onClick={loadFiles} title="Reload list">
            ↻
          </Button>
        </div>
        <ul className="flex-1 overflow-y-auto px-2 pb-3">
          {shown.map((f) => (
            <li key={f.path}>
              <button
                onClick={() => select(f.path)}
                className={`flex w-full flex-col rounded-md px-3 py-2 text-left ${
                  f.path === selected ? "bg-zinc-800" : "hover:bg-zinc-800/50"
                }`}
              >
                <span className="truncate text-sm">{f.plugin || baseName(f.path)}</span>
                <span className="truncate text-xs text-zinc-500" title={f.path}>
                  {f.path}
                </span>
              </button>
            </li>
          ))}
          {shown.length === 0 && <p className="px-3 py-6 text-center text-sm text-zinc-500">No config files.</p>}
        </ul>
        <p className="border-t border-zinc-800 px-3 py-2 text-xs text-zinc-500">
          Most mods create their config the first time the game runs with them.
        </p>
      </aside>
      <div className="min-w-0 flex-1">
        <ErrorText>{error}</ErrorText>
        {selected ? (
          <ConfigEditor key={selected} path={selected} onDirtyChange={setDirty} />
        ) : (
          <div className="flex h-full items-center justify-center text-sm text-zinc-500">Select a config file</div>
        )}
      </div>
    </div>
  );
}

// BepInEx/config .cfg files first, then everything else, by display name.
function sortFiles(files: ConfigFileInfo[]): ConfigFileInfo[] {
  const rank = (f: ConfigFileInfo) => (f.path.startsWith("BepInEx/config/") && f.path.endsWith(".cfg") ? 0 : 1);
  return [...files].sort(
    (a, b) => rank(a) - rank(b) || (a.plugin || baseName(a.path)).localeCompare(b.plugin || baseName(b.path)),
  );
}

function baseName(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1);
}

const entryKey = (section: string, key: string) => JSON.stringify([section, key]);

function ConfigEditor({ path, onDirtyChange }: { path: string; onDirtyChange: (dirty: boolean) => void }) {
  const { game, profile } = useProfile();
  const [content, setContent] = useState<ConfigContent | null>(null);
  const [mode, setMode] = useState<"settings" | "text">("settings");
  const [values, setValues] = useState<Map<string, string>>(new Map());
  const [text, setText] = useState("");
  const [running, setRunning] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback((c: ConfigContent) => {
    setContent(c);
    setValues(new Map());
    setText(c.text);
    if (!c.document) setMode("text");
  }, []);

  useEffect(() => {
    ConfigService.ReadConfig(game.id, profile.id, path)
      .then(load)
      .catch((err) => setError(errorMessage(err)));
    LaunchService.GetLaunchInfo(game.id)
      .then((info) => setRunning(info.running))
      .catch(() => {});
  }, [game.id, profile.id, path, load]);

  const entries = useMemo(() => {
    const all = new Map<string, { section: string; entry: Entry }>();
    for (const s of content?.document?.sections ?? []) {
      for (const e of s.entries ?? []) all.set(entryKey(s.name, e.key), { section: s.name, entry: e });
    }
    return all;
  }, [content]);

  const changes: Change[] = [...values.entries()]
    .filter(([k, v]) => entries.get(k)?.entry.value !== v)
    .map(([k, value]) => {
      const { section, entry } = entries.get(k)!;
      return { section, key: entry.key, line: entry.line, value };
    });
  const invalid = changes.some((c) => validate(entries.get(entryKey(c.section, c.key))!.entry, c.value) !== "");
  const dirty = mode === "text" ? text !== content?.text : changes.length > 0;

  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange]);

  const save = async () => {
    setError("");
    setSaving(true);
    try {
      load(
        mode === "text"
          ? await ConfigService.SaveConfigText(game.id, profile.id, path, text)
          : await ConfigService.SaveConfigChanges(game.id, profile.id, path, changes),
      );
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const revert = () => {
    setValues(new Map());
    setText(content?.text ?? "");
  };

  const switchMode = (next: "settings" | "text") => {
    if (next === mode) return;
    if (dirty) {
      setError("Save or revert your changes before switching the view.");
      return;
    }
    setError("");
    setMode(next);
  };

  if (!content) {
    return (
      <div className="p-6">
        {error ? <ErrorText>{error}</ErrorText> : <p className="text-sm text-zinc-500">Loading…</p>}
      </div>
    );
  }
  const header = content.document?.header ?? [];
  const sections = content.document?.sections ?? [];

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-3 border-b border-zinc-800 px-6 py-3">
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="truncate font-medium">{baseName(path)}</span>
          <span className="truncate text-xs text-zinc-500">{path}</span>
        </div>
        {content.document && (
          <div className="flex rounded-md bg-zinc-800 p-0.5 text-xs">
            {(["settings", "text"] as const).map((m) => (
              <button
                key={m}
                onClick={() => switchMode(m)}
                className={`rounded px-2.5 py-1 capitalize ${mode === m ? "bg-zinc-600 text-white" : "text-zinc-400"}`}
              >
                {m}
              </button>
            ))}
          </div>
        )}
      </div>

      {running && (
        <p className="border-b border-amber-500/20 bg-amber-500/5 px-6 py-2 text-xs text-amber-400">
          The game is running. Mods may overwrite their config when the game exits; edit configs while it is closed.
        </p>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto">
        {mode === "text" ? (
          <textarea
            className="h-full w-full resize-none bg-transparent p-6 font-mono text-xs leading-relaxed text-zinc-200 outline-none select-text"
            spellCheck={false}
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
        ) : (
          <div className="flex flex-col gap-6 px-6 py-4">
            {header.length > 0 && <p className="text-xs whitespace-pre-line text-zinc-500">{header.join("\n")}</p>}
            {sections.map((s) => (
              <section key={s.name} className="flex flex-col gap-2">
                <h3 className="text-sm font-semibold text-zinc-300">{s.name}</h3>
                <div className="divide-y divide-zinc-800 rounded-lg border border-zinc-800">
                  {(s.entries ?? []).map((e) => {
                    const k = entryKey(s.name, e.key);
                    return (
                      <EntryRow
                        key={k}
                        entry={e}
                        value={values.get(k) ?? e.value}
                        onChange={(v) => setValues((prev) => new Map(prev).set(k, v))}
                      />
                    );
                  })}
                </div>
              </section>
            ))}
            {sections.length === 0 && <p className="text-sm text-zinc-500">This file has no settings.</p>}
          </div>
        )}
      </div>

      <div className="flex items-center gap-3 border-t border-zinc-800 px-6 py-3">
        <span className="flex-1 text-xs text-zinc-500">
          {dirty &&
            (mode === "settings"
              ? `${changes.length} unsaved ${changes.length === 1 ? "change" : "changes"}`
              : "Unsaved changes")}
        </span>
        <ErrorText>{error}</ErrorText>
        <Button variant="ghost" disabled={!dirty || saving} onClick={revert}>
          Revert
        </Button>
        <Button variant="primary" disabled={!dirty || saving || (mode === "settings" && invalid)} onClick={save}>
          {saving ? "Saving…" : "Save"}
        </Button>
      </div>
    </div>
  );
}

const integerTypes = ["Byte", "SByte", "Int16", "UInt16", "Int32", "UInt32", "Int64", "UInt64"];
const floatTypes = ["Single", "Double", "Decimal"];

// validate returns a problem with a value, or "".
function validate(entry: Entry, value: string): string {
  const isInt = integerTypes.includes(entry.type);
  if (!isInt && !floatTypes.includes(entry.type)) return "";
  const n = Number(value.trim());
  if (value.trim() === "" || !Number.isFinite(n)) return "Enter a number";
  if (isInt && !Number.isInteger(n)) return "Enter a whole number";
  if (entry.hasRange && (n < entry.min || n > entry.max)) return `Must be between ${entry.min} and ${entry.max}`;
  return "";
}

function EntryRow({ entry, value, onChange }: { entry: Entry; value: string; onChange: (value: string) => void }) {
  const problem = validate(entry, value);
  const options = entry.acceptableValues ?? [];
  const isDefault = !entry.hasDefault || value.trim() === entry.default.trim();

  let input;
  if (entry.type === "Boolean") {
    input = <Toggle checked={value.toLowerCase() === "true"} onChange={(on) => onChange(on ? "true" : "false")} />;
  } else if (options.length > 0 && entry.multiple) {
    const chosen = value
      .split(",")
      .map((v) => v.trim())
      .filter(Boolean);
    input = (
      <div className="flex flex-wrap gap-1.5">
        {options.map((o) => {
          const on = chosen.includes(o);
          return (
            <button
              key={o}
              onClick={() => onChange((on ? chosen.filter((c) => c !== o) : [...chosen, o]).join(", "))}
              className={`rounded px-2 py-0.5 text-xs ${
                on ? "bg-accent-500 text-zinc-950" : "bg-zinc-800 text-zinc-400 hover:text-zinc-200"
              }`}
            >
              {o}
            </button>
          );
        })}
      </div>
    );
  } else if (options.length > 0) {
    input = (
      <select className={`${inputClass} w-auto`} value={value} onChange={(e) => onChange(e.target.value)}>
        {!options.includes(value) && <option value={value}>{value}</option>}
        {options.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </select>
    );
  } else {
    const numeric = integerTypes.includes(entry.type) || floatTypes.includes(entry.type);
    input = (
      <input
        className={`${inputClass} ${numeric ? "w-32" : ""} ${problem ? "border-red-500" : ""}`}
        value={value}
        inputMode={numeric ? "decimal" : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  }

  return (
    <div className="flex flex-col gap-2 px-4 py-3">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-sm font-medium select-text">{entry.key}</span>
        <span className="shrink-0 text-xs text-zinc-500">
          {entry.type}
          {entry.hasRange && ` · ${entry.min}–${entry.max}`}
        </span>
      </div>
      {entry.description && (
        <p className="text-xs whitespace-pre-line text-zinc-400 select-text">{entry.description}</p>
      )}
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">{input}</div>
        {!isDefault && (
          <button className="shrink-0 text-xs text-zinc-500 hover:text-zinc-200" onClick={() => onChange(entry.default)}>
            Reset to default ({entry.default || "empty"})
          </button>
        )}
      </div>
      {problem && <p className="text-xs text-red-400">{problem}</p>}
    </div>
  );
}
