import { useState } from "react";
import { errorMessage, PreparedMod } from "../api";
import { Button, ErrorText, inputClass, Modal } from "./ui";

type Props = {
  prepared: PreparedMod;
  // onChoose goes on with the install: replace, merge, or rename with a name.
  onChoose: (target: "replace" | "merge" | "rename", name: string) => Promise<void>;
  onCancel: () => void;
};

// DuplicateModModal asks what to do with an archive whose mod the profile
// has already, the way MO2 asks: replace it, merge with it, install this one
// under another name, or not at all.
export default function DuplicateModModal({ prepared, onChoose, onCancel }: Props) {
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(`${prepared.name} 2`);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const choose = async (target: "replace" | "merge" | "rename") => {
    setBusy(true);
    setError("");
    try {
      await onChoose(target, name.trim());
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  };

  const options: { target: "replace" | "merge"; label: string; text: string }[] = [
    { target: "replace", label: "Replace", text: "Its files go and this one's take their place." },
    { target: "merge", label: "Merge", text: "This one's files go over it; files only it has stay." },
  ];

  return (
    <Modal title="Mod already installed" onClose={busy ? () => {} : onCancel}>
      <div className="flex flex-col gap-4">
        <p className="text-sm text-zinc-300">
          The profile has a mod named <span className="font-semibold">{prepared.existing}</span> already. Replacing or
          merging keeps its place in the list and whether it is on.
        </p>
        {renaming ? (
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-zinc-400">Install this one as</span>
            <input
              autoFocus
              className={inputClass}
              value={name}
              disabled={busy}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && name.trim() && choose("rename")}
            />
          </label>
        ) : (
          <div className="flex flex-col gap-2">
            {options.map((o) => (
              <button
                key={o.target}
                disabled={busy}
                onClick={() => choose(o.target)}
                className="flex flex-col rounded-lg border border-zinc-800 px-4 py-2.5 text-left hover:border-zinc-700 hover:bg-zinc-800/60 disabled:opacity-50"
              >
                <span className="text-sm font-medium">{o.label}</span>
                <span className="text-xs text-zinc-500">{o.text}</span>
              </button>
            ))}
            <button
              disabled={busy}
              onClick={() => setRenaming(true)}
              className="flex flex-col rounded-lg border border-zinc-800 px-4 py-2.5 text-left hover:border-zinc-700 hover:bg-zinc-800/60 disabled:opacity-50"
            >
              <span className="text-sm font-medium">Rename</span>
              <span className="text-xs text-zinc-500">Install this one as a mod of its own, under another name.</span>
            </button>
          </div>
        )}
        <ErrorText>{error}</ErrorText>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" disabled={busy} onClick={renaming ? () => setRenaming(false) : onCancel}>
            {renaming ? "Back" : "Cancel"}
          </Button>
          {renaming && (
            <Button variant="primary" disabled={busy || !name.trim()} onClick={() => choose("rename")}>
              Install
            </Button>
          )}
        </div>
      </div>
    </Modal>
  );
}
