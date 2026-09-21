import { useState } from "react";
import { Dialogs } from "@wailsio/runtime";
import {
  confirmDanger,
  errorMessage,
  GitHubAsset,
  GitHubRelease,
  GitHubRepo,
  InstallService,
  LocalPackage,
  Profile,
} from "../../api";
import { formatAgo, formatBytes } from "../../format";
import { Button, ErrorText, inputClass, Modal } from "../ui";
import { conflictMessage, useProfile } from "./ProfileContext";

type Tab = "file" | "github";

// Where the package being installed came from; GitHub mods can be updated later.
type Origin = { kind: "file" } | { kind: "github"; owner: string; repo: string; tag: string; asset: string };

export default function AddModModal({ onClose }: { onClose: () => void }) {
  const [tab, setTab] = useState<Tab>("file");
  const [pkg, setPkg] = useState<LocalPackage | null>(null);
  const [origin, setOrigin] = useState<Origin>({ kind: "file" });

  const tabs: { id: Tab; label: string }[] = [
    { id: "file", label: "From file" },
    { id: "github", label: "From GitHub" },
  ];

  return (
    <Modal title="Add a mod" size="lg" onClose={onClose}>
      {pkg ? (
        <PackageForm pkg={pkg} origin={origin} onBack={() => setPkg(null)} onInstalled={onClose} />
      ) : (
        <>
          <nav className="mb-4 flex gap-1 border-b border-zinc-800">
            {tabs.map((t) => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className={`-mb-px border-b-2 px-3 pb-2 text-sm ${
                  tab === t.id
                    ? "border-indigo-500 text-zinc-100"
                    : "border-transparent text-zinc-400 hover:text-zinc-200"
                }`}
              >
                {t.label}
              </button>
            ))}
          </nav>
          {tab === "file" ? (
            <FilePicker
              onPicked={(p) => {
                setOrigin({ kind: "file" });
                setPkg(p);
              }}
            />
          ) : (
            <GitHubPicker
              onPicked={(p, o) => {
                setOrigin(o);
                setPkg(p);
              }}
            />
          )}
        </>
      )}
    </Modal>
  );
}

function FilePicker({ onPicked }: { onPicked: (pkg: LocalPackage) => void }) {
  const [error, setError] = useState("");

  const pick = async () => {
    setError("");
    try {
      const path = await Dialogs.OpenFile({
        Title: "Choose a mod file",
        CanChooseFiles: true,
        Filters: [{ DisplayName: "Mod files (*.zip, *.dll)", Pattern: "*.zip;*.dll" }],
      });
      if (path) onPicked(await InstallService.InspectFile(path));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <div className="flex flex-col gap-3 text-sm">
      <p className="text-zinc-400">
        A mod archive (.zip, with or without a Thunderstore manifest) or a single plugin .dll. Hermit reads the plugin
        name and version from the DLL.
      </p>
      <Button className="self-start" onClick={pick}>
        Choose file…
      </Button>
      <ErrorText>{error}</ErrorText>
    </div>
  );
}

function GitHubPicker({ onPicked }: { onPicked: (pkg: LocalPackage, origin: Origin) => void }) {
  const [input, setInput] = useState("");
  const [repo, setRepo] = useState<GitHubRepo | null>(null);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  const find = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy("releases");
    try {
      setRepo(await InstallService.GitHubReleases(input));
    } catch (err) {
      setRepo(null);
      setError(errorMessage(err));
    } finally {
      setBusy("");
    }
  };

  const download = async (release: GitHubRelease, asset: GitHubAsset) => {
    if (!repo) return;
    setError("");
    setBusy(asset.browser_download_url);
    try {
      const pkg = await InstallService.DownloadGitHubAsset(repo.owner, repo.repo, release.tag_name, asset);
      onPicked(pkg, { kind: "github", owner: repo.owner, repo: repo.repo, tag: release.tag_name, asset: asset.name });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy("");
    }
  };

  const isModFile = (name: string) => /\.(zip|dll)$/i.test(name);

  return (
    <div className="flex flex-col gap-3 text-sm">
      <form onSubmit={find} className="flex gap-2">
        <input
          className={inputClass}
          placeholder="owner/repo or https://github.com/owner/repo"
          value={input}
          autoFocus
          onChange={(e) => setInput(e.target.value)}
        />
        <Button type="submit" disabled={!input.trim() || busy !== ""}>
          {busy === "releases" ? "Loading…" : "Find releases"}
        </Button>
      </form>
      <ErrorText>{error}</ErrorText>
      {repo && (
        <ul className="flex max-h-96 flex-col gap-3 overflow-y-auto">
          {(repo.releases ?? []).map((r) => (
            <li key={r.tag_name} className="rounded-lg border border-zinc-800 p-3">
              <div className="mb-2 flex items-baseline gap-2">
                <span className="font-medium">{r.name || r.tag_name}</span>
                <span className="text-xs text-zinc-500">{r.tag_name}</span>
                {r.prerelease && <span className="text-xs text-amber-400">pre-release</span>}
                <span className="ml-auto text-xs text-zinc-500">{formatAgo(r.published_at)}</span>
              </div>
              <ul className="flex flex-col gap-1">
                {(r.assets ?? []).map((a) => (
                  <li key={a.name} className="flex items-center gap-3">
                    <span className={`min-w-0 flex-1 truncate ${isModFile(a.name) ? "" : "text-zinc-500"}`}>
                      {a.name}
                    </span>
                    <span className="text-xs text-zinc-500">{formatBytes(a.size)}</span>
                    <Button
                      variant={isModFile(a.name) ? "secondary" : "ghost"}
                      disabled={busy !== "" || !isModFile(a.name)}
                      onClick={() => download(r, a)}
                    >
                      {busy === a.browser_download_url ? "Downloading…" : "Use"}
                    </Button>
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

type FormProps = {
  pkg: LocalPackage;
  origin: Origin;
  onBack: () => void;
  onInstalled: () => void;
};

function PackageForm({ pkg, origin, onBack, onInstalled }: FormProps) {
  const { game, profile, applyProfile } = useProfile();
  const [author, setAuthor] = useState(pkg.author);
  const [name, setName] = useState(pkg.name);
  const [version, setVersion] = useState(pkg.version);
  const [targetDir, setTargetDir] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const valid = /^[A-Za-z0-9_-]+$/.test(author) && /^[A-Za-z0-9_]+$/.test(name) && /^\d+\.\d+\.\d+$/.test(version);
  const id = `${author}-${name}`;
  const existing = profile.mods?.find((m) => m.id === id);
  const plugins = pkg.plugins ?? [];
  const dependencies = pkg.dependencies ?? [];

  const install = async () => {
    setError("");
    setBusy(true);
    const edited = { ...pkg, author, name, version, targetDir: targetDir.trim() };
    try {
      const plan = await InstallService.PlanFile(game.id, profile.id, edited);
      const conflicts = plan.conflicts ?? [];
      if (conflicts.length > 0 && !(await confirmDanger("Conflicting mods", conflictMessage(conflicts), "Replace"))) {
        return;
      }
      const replace = conflicts.length > 0;
      let result: Profile;
      if (origin.kind === "github") {
        result = await InstallService.InstallGitHub(
          game.id,
          profile.id,
          edited,
          origin.owner,
          origin.repo,
          origin.tag,
          origin.asset,
          replace,
        );
      } else {
        result = await InstallService.InstallFile(game.id, profile.id, edited, replace);
      }
      applyProfile(result);
      onInstalled();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-4 text-sm">
      <p className="truncate text-zinc-400" title={pkg.path}>
        {origin.kind === "github" ? `${origin.owner}/${origin.repo} · ${origin.tag} · ${origin.asset}` : pkg.path}
      </p>

      {plugins.length > 0 ? (
        <div className="flex flex-col gap-1 rounded-lg border border-zinc-800 p-3">
          <span className="text-xs text-zinc-500">BepInEx plugins found</span>
          {plugins.map((p) => (
            <span key={p.guid}>
              {p.name} <span className="text-zinc-500">v{p.version}</span>{" "}
              <span className="text-xs text-zinc-500 select-text">{p.guid}</span>
            </span>
          ))}
        </div>
      ) : (
        <p className="text-amber-400">No BepInEx plugin found in this file. It may be a library, a patcher or assets.</p>
      )}

      <div className="grid grid-cols-3 gap-3">
        <label className="flex flex-col gap-1.5">
          <span className="text-zinc-400">Author</span>
          <input className={inputClass} value={author} onChange={(e) => setAuthor(e.target.value)} />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-zinc-400">Name</span>
          <input className={inputClass} value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-zinc-400">Version</span>
          <input className={inputClass} value={version} onChange={(e) => setVersion(e.target.value)} />
        </label>
      </div>
      <p className="text-xs text-zinc-500">
        Installed as <span className="text-zinc-300 select-text">{id}</span>
        {existing && ` — replaces the installed version ${existing.version}`}. Letters, digits and _ only; version like
        1.2.3.
      </p>

      <label className="flex flex-col gap-1.5">
        <span className="text-zinc-400">Install into (optional)</span>
        <input
          className={inputClass}
          placeholder="Where the install rules put it — for plugins, BepInEx/plugins"
          value={targetDir}
          onChange={(e) => setTargetDir(e.target.value)}
        />
        <span className="text-xs text-zinc-500">
          {plugins.length === 0
            ? "This file has no BepInEx plugin, so it is probably files another mod reads. Its page usually names the folder they go into, like BepInEx/plugins/models/all — put that here. The folders are created, the game does not have to run first."
            : "A folder of the game, for mods whose page says to put their files somewhere specific. The folders are created as needed."}
        </span>
      </label>

      {dependencies.length > 0 && (
        <p className="text-xs text-zinc-400">Dependencies, installed from Thunderstore if missing: {dependencies.join(", ")}</p>
      )}

      <ErrorText>{error}</ErrorText>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" disabled={busy} onClick={onBack}>
          Back
        </Button>
        <Button variant="primary" disabled={busy || !valid} onClick={install}>
          {busy ? "Installing…" : "Install"}
        </Button>
      </div>
    </div>
  );
}
