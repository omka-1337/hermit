import { useEffect, useState } from "react";
import { Clipboard, Dialogs, Events } from "@wailsio/runtime";
import { errorMessage, Game, ImportProgress, ImportResult, Preview, Profile, ShareService } from "../api";
import { Button, ErrorText, inputClass, Modal } from "./ui";

const r2zFilter = [{ DisplayName: "r2modman profile (*.r2z)", Pattern: "*.r2z" }];

export function ExportModal({ game, profile, onClose }: { game: Game; profile: Profile; onClose: () => void }) {
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const run = async (action: () => Promise<void>) => {
    setError("");
    setMessage("");
    setBusy(true);
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const saveFile = () =>
    run(async () => {
      const dest = await Dialogs.SaveFile({
        Title: "Export profile",
        Filename: `${profile.name}.r2z`,
        Filters: r2zFilter,
        CanCreateDirectories: true,
      });
      if (!dest) return;
      await ShareService.ExportFile(game.id, profile.id, dest.endsWith(".r2z") ? dest : `${dest}.r2z`);
      setMessage("Profile saved.");
    });

  const shareCode = () => run(async () => setCode(await ShareService.ExportCode(game.id, profile.id)));

  return (
    <Modal title={`Export “${profile.name}”`} onClose={onClose}>
      <div className="flex flex-col gap-4 text-sm">
        <p className="text-zinc-400">
          The export contains the mod list and config files, in the same format as r2modman, so it can be imported
          there too.
        </p>
        <div className="flex flex-col gap-2 rounded-lg border border-zinc-800 p-3">
          <span className="font-medium">File</span>
          <span className="text-xs text-zinc-500">Save an .r2z file to send it any way you like.</span>
          <Button className="self-start" disabled={busy} onClick={saveFile}>
            Save as file…
          </Button>
        </div>
        <div className="flex flex-col gap-2 rounded-lg border border-zinc-800 p-3">
          <span className="font-medium">Code</span>
          <span className="text-xs text-zinc-500">
            Uploads the profile to Thunderstore. Anyone who has the code can download it.
          </span>
          {code ? (
            <div className="flex gap-2">
              <code className="flex-1 rounded-md bg-zinc-950 px-3 py-1.5 font-mono text-xs select-text">{code}</code>
              <Button onClick={() => Clipboard.SetText(code).then(() => setMessage("Code copied."))}>Copy</Button>
            </div>
          ) : (
            <Button className="self-start" disabled={busy} onClick={shareCode}>
              {busy ? "Uploading…" : "Get share code"}
            </Button>
          )}
        </div>
        {message && <p className="text-emerald-400">{message}</p>}
        <ErrorText>{error}</ErrorText>
        <div className="flex justify-end">
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </Modal>
  );
}

type ImportProps = {
  game: Game;
  onClose: () => void;
  onImported: (profileId: string) => void;
  // from narrows the start to one way in, an .r2z file or a profile code;
  // without it both are offered.
  from?: "file" | "code";
  // onBack, when set, goes back from the start to where the dialog came from.
  onBack?: () => void;
};

export function ImportModal({ game, onClose, onImported, from, onBack }: ImportProps) {
  const [code, setCode] = useState("");
  const [preview, setPreview] = useState<Preview | null>(null);
  const [name, setName] = useState("");
  const [progress, setProgress] = useState<ImportProgress | null>(null);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(
    () =>
      Events.On("import:progress", (ev) => {
        if (ev.data.gameId === game.id) setProgress(ev.data);
      }),
    [game.id],
  );

  const load = async (file: string, code: string) => {
    setError("");
    setBusy(true);
    try {
      const p = await ShareService.PreviewImport({ file, code });
      setPreview(p);
      setName(p.profileName);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const pickFile = async () => {
    const file = await Dialogs.OpenFile({ Title: "Import profile", Filters: r2zFilter, CanChooseFiles: true });
    if (file) await load(file, "");
  };

  const runImport = async () => {
    if (!preview) return;
    setError("");
    setBusy(true);
    try {
      setResult(await ShareService.Import(game.id, preview.archive, name.trim()));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const codeValid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(code.trim());
  const mods = preview?.mods ?? [];

  let body;
  if (result) {
    const failed = result.failed ?? [];
    body = (
      <>
        <p className="text-emerald-400">
          Imported “{result.profile.name}” with {result.profile.mods?.length ?? 0} mods.
        </p>
        {failed.length > 0 && (
          <div className="flex flex-col gap-1">
            <p className="text-amber-400">{failed.length} could not be installed:</p>
            <ul className="max-h-40 overflow-y-auto text-xs text-zinc-400 select-text">
              {failed.map((f) => (
                <li key={f.package}>
                  <span className="text-zinc-200">{f.package}</span>: {f.error}
                </li>
              ))}
            </ul>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
          <Button variant="primary" onClick={() => onImported(result.profile.id)}>
            Open profile
          </Button>
        </div>
      </>
    );
  } else if (preview) {
    body = (
      <>
        <label className="flex flex-col gap-1.5">
          <span className="text-zinc-400">Profile name</span>
          <input className={inputClass} value={name} onChange={(e) => setName(e.target.value)} disabled={busy} />
        </label>
        <div className="flex flex-col gap-1.5">
          <span className="text-zinc-400">{mods.length} mods</span>
          <ul className="max-h-56 divide-y divide-zinc-800 overflow-y-auto rounded-md border border-zinc-800 text-xs">
            {mods.map((m) => (
              <li key={m.name} className={`flex justify-between px-3 py-1.5 ${m.enabled ? "" : "text-zinc-500"}`}>
                <span>
                  {m.name}
                  {!m.enabled && " (disabled)"}
                </span>
                <span className="text-zinc-500">
                  {m.version.major}.{m.version.minor}.{m.version.patch}
                </span>
              </li>
            ))}
          </ul>
        </div>
        {busy && progress && (
          <p className="text-xs text-zinc-400">
            Installing {progress.package || "configs"} ({Math.min(progress.done + 1, progress.total)}/{progress.total})…
          </p>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" disabled={busy} onClick={() => setPreview(null)}>
            Back
          </Button>
          <Button variant="primary" disabled={busy || !name.trim()} onClick={runImport}>
            {busy ? "Importing…" : "Import"}
          </Button>
        </div>
      </>
    );
  } else {
    body = (
      <>
        {from !== "code" && (
          <>
            <p className="text-zinc-400">Import a profile exported from this manager or from r2modman.</p>
            <Button className="self-start" disabled={busy} onClick={pickFile}>
              Choose .r2z file…
            </Button>
          </>
        )}
        {from !== "file" && (
          <div className="flex flex-col gap-1.5">
            <span className="text-zinc-400">
              {from === "code" ? "A profile code someone shared from r2modman or Hermit" : "or enter a profile code"}
            </span>
            <div className="flex gap-2">
              <input
                className={`${inputClass} font-mono`}
                placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                value={code}
                onChange={(e) => setCode(e.target.value)}
              />
              <Button disabled={busy || !codeValid} onClick={() => load("", code.trim())}>
                {busy ? "Loading…" : "Load"}
              </Button>
            </div>
          </div>
        )}
        <div className="flex justify-end gap-2">
          {onBack && (
            <Button variant="ghost" onClick={onBack}>
              Back
            </Button>
          )}
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </>
    );
  }

  return (
    <Modal title={from === "code" ? "Download a profile from its code" : "Import profile"} onClose={busy ? () => {} : onClose}>
      <div className="flex flex-col gap-4 text-sm">
        {body}
        <ErrorText>{error}</ErrorText>
      </div>
    </Modal>
  );
}
