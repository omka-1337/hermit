import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import {
  confirmDanger,
  Conflict,
  ConflictReason,
  Game,
  InstallService,
  LaunchService,
  Mod,
  Profile,
  Progress,
  Report,
  UpdateResult,
} from "../../api";
import { compareVersions } from "../../format";

type ProfileState = {
  game: Game;
  profile: Profile;
  installed: Map<string, Mod>;
  // busy is the mod id currently being installed or removed; one at a time per profile.
  busy: string | null;
  progress: Progress | null;
  // report describes the last game session of this profile, if any.
  report: Report | null;
  // modpack installs exact dependency versions.
  install: (namespace: string, name: string, version: string, modpack: boolean) => Promise<void>;
  // openProfile switches to another profile of the game.
  openProfile: (profileId: string) => void;
  // applyProfile shows a profile returned by an operation done elsewhere.
  applyProfile: (profile: Profile) => void;
  uninstall: (modId: string) => Promise<void>;
  setEnabled: (modId: string, enabled: boolean) => Promise<void>;
  // updates maps ids of outdated mods to their latest version.
  updates: Map<string, string>;
  checkingUpdates: boolean;
  // checked is set once a check has finished, so "up to date" means it.
  checked: boolean;
  checkUpdates: () => Promise<void>;
  // updating is the mod being updated right now, updated the ones already
  // brought up to date in this session, and queued the ones an "Update all"
  // has still to reach.
  updating: string | null;
  updated: Set<string>;
  queued: Set<string>;
  update: (modId: string) => Promise<void>;
  updateAll: () => Promise<UpdateResult | null>;
};

const ProfileContext = createContext<ProfileState | null>(null);

export function useProfile(): ProfileState {
  const ctx = useContext(ProfileContext);
  if (!ctx) throw new Error("useProfile must be used inside ProfileProvider");
  return ctx;
}

type Props = {
  game: Game;
  profile: Profile;
  onProfileChange: (profile: Profile) => void;
  // onOpenProfile switches to another profile, e.g. one created from a modpack.
  onOpenProfile: (profileId: string) => void;
  children: React.ReactNode;
};

export function ProfileProvider({ game, profile, onProfileChange, onOpenProfile, children }: Props) {
  const [busy, setBusy] = useState<string | null>(null);
  const [progress, setProgress] = useState<Progress | null>(null);
  const [report, setReport] = useState<Report | null>(null);
  // latest holds the newest known version of each mod, from the last check.
  const [latest, setLatest] = useState<Map<string, string>>(new Map());
  const [checkingUpdates, setCheckingUpdates] = useState(false);
  const [checked, setChecked] = useState(false);
  const [updatingOne, setUpdatingOne] = useState<string | null>(null);
  const [updated, setUpdated] = useState<Set<string>>(new Set());
  const [queued, setQueued] = useState<Set<string>>(new Set());

  const markUpdated = useCallback((ids: string[]) => {
    if (ids.length === 0) return;
    setUpdated((done) => new Set([...done, ...ids]));
  }, []);

  const checkUpdates = useCallback(async () => {
    setCheckingUpdates(true);
    try {
      const found = (await InstallService.CheckUpdates(game.id, profile.id)) ?? [];
      setLatest(new Map(found.map((u) => [u.modId, u.latest])));
      setChecked(true);
    } finally {
      setCheckingUpdates(false);
    }
  }, [game.id, profile.id]);

  useEffect(() => {
    checkUpdates().catch(console.error);
  }, [checkUpdates]);

  // The game runs in a separate wrapper process; poll to notice when a session
  // ends, then pick up its report and any state changes.
  useEffect(() => {
    let wasRunning = false;
    const check = async () => {
      try {
        const info = await LaunchService.GetLaunchInfo(game.id);
        if (wasRunning && !info.running) {
          onProfileChange(await InstallService.OpenProfile(game.id, profile.id));
        }
        wasRunning = info.running;
        setReport(await LaunchService.GetLaunchReport(game.id, profile.id));
      } catch (err) {
        console.error(err);
      }
    };
    check();
    const timer = setInterval(check, 5000);
    return () => clearInterval(timer);
  }, [game.id, profile.id, onProfileChange]);

  useEffect(
    () =>
      Events.On("install:progress", (ev) => {
        const p = ev.data;
        if (p.gameId === game.id && p.profileId === profile.id) setProgress(p);
      }),
    [game.id, profile.id],
  );

  const updatingAll = useRef<string | null>(null);
  useEffect(() => {
    if (busy !== "update-all") {
      updatingAll.current = null;
      return;
    }
    const target = progress?.target ?? null;
    if (!target) return;
    // The previous target is finished once another package starts.
    if (updatingAll.current && updatingAll.current !== target) markUpdated([updatingAll.current]);
    updatingAll.current = target;
  }, [busy, progress, markUpdated]);

  const run = useCallback(
    async (id: string, action: () => Promise<Profile>) => {
      setBusy(id);
      setProgress(null);
      try {
        onProfileChange(await action());
      } finally {
        setBusy(null);
        setProgress(null);
      }
    },
    [onProfileChange],
  );

  const updates = useMemo(() => {
    const outdated = new Map<string, string>();
    for (const m of profile.mods ?? []) {
      const newest = latest.get(m.id);
      if (newest && compareVersions(newest, m.version) > 0) outdated.set(m.id, newest);
    }
    return outdated;
  }, [latest, profile.mods]);

  const install = useCallback(
    (namespace: string, name: string, version: string, modpack: boolean) =>
      run(`${namespace}-${name}`, async () => {
        const plan = await InstallService.PlanInstall(game.id, profile.id, namespace, name, version, {
          modpack,
          replaceConflicts: false,
        });
        const conflicts = plan.conflicts ?? [];
        const blocking = conflicts.find((c) => c.blocking);
        if (blocking) {
          throw new Error(`${packageName(blocking.package)} cannot be installed together with ${blocking.modName}.`);
        }
        if (conflicts.length > 0) {
          const ok = await confirmDanger("Conflicting mods", conflictMessage(conflicts), "Replace");
          if (!ok) return profile;
        }
        return InstallService.InstallPackage(game.id, profile.id, namespace, name, version, {
          modpack,
          replaceConflicts: conflicts.length > 0,
        });
      }),
    [game.id, profile, run],
  );

  const value = useMemo<ProfileState>(
    () => ({
      game,
      profile,
      installed: new Map((profile.mods ?? []).map((m) => [m.id, m])),
      busy,
      progress,
      report,
      install,
      openProfile: onOpenProfile,
      applyProfile: onProfileChange,
      uninstall: (modId) => run(modId, () => InstallService.UninstallMod(game.id, profile.id, modId)),
      setEnabled: (modId, enabled) =>
        run(modId, () => InstallService.SetModEnabled(game.id, profile.id, modId, enabled)),
      updates,
      checkingUpdates,
      checked,
      checkUpdates,
      updating: busy === "update-all" ? (progress?.target ?? null) : updatingOne,
      updated,
      queued,
      update: async (modId) => {
        setUpdatingOne(modId);
        try {
          await run(modId, () => InstallService.UpdateMod(game.id, profile.id, modId));
          markUpdated([modId]);
        } finally {
          setUpdatingOne(null);
        }
      },
      updateAll: async () => {
        let result: UpdateResult | null = null;
        let done: string[] = [];
        setQueued(new Set(updates.keys()));
        try {
          await run("update-all", async () => {
            const outcome = await InstallService.UpdateAll(game.id, profile.id);
            result = outcome;
            done = outcome.updated ?? [];
            return outcome.profile;
          });
        } finally {
          setQueued(new Set());
        }
        markUpdated(done);
        // A mod is marked as done when the next one starts, which also marks
        // the ones that failed; the result says which those were.
        const failed = Object.keys((result as UpdateResult | null)?.failed ?? {});
        if (failed.length) {
          setUpdated((ids) => new Set([...ids].filter((id) => !failed.includes(id))));
        }
        return result;
      },
    }),
    [
      game,
      profile,
      busy,
      progress,
      report,
      run,
      install,
      updates,
      checkingUpdates,
      checked,
      checkUpdates,
      updatingOne,
      updated,
      queued,
      markUpdated,
      onOpenProfile,
      onProfileChange,
    ],
  );

  return <ProfileContext.Provider value={value}>{children}</ProfileContext.Provider>;
}

// dependantsOf returns installed mods that declare a dependency on modId.
export function dependantsOf(installed: Map<string, Mod>, modId: string): Mod[] {
  return [...installed.values()].filter((m) =>
    (m.dependencies ?? []).some((d) => d.startsWith(`${modId}-`) && !d.slice(modId.length + 1).includes("-")),
  );
}

// dependencyLabel describes an unmet dependency string "<author>-<name>-<version>".
export function dependencyLabel(installed: Map<string, Mod>, dep: string): string {
  const id = dep.replace(/-\d+\.\d+\.\d+$/, "");
  const name = id.slice(id.lastIndexOf("-") + 1);
  return installed.has(id) ? `${name} (disabled)` : `${name} (not installed)`;
}

export function thunderstoreIconURL(mod: Mod): string {
  return `https://gcdn.thunderstore.io/live/repository/icons/${mod.id}-${mod.version}.png`;
}

function packageName(pkg: string): string {
  const id = pkg.replace(/-\d+\.\d+\.\d+$/, "");
  return id.slice(id.lastIndexOf("-") + 1);
}

export function conflictMessage(conflicts: Conflict[]): string {
  const lines = conflicts.map((c) =>
    c.reason === ConflictReason.ConflictLoader
      ? `• ${packageName(c.package)} is a BepInEx loader, only one can be installed: ${c.modName} will be uninstalled.`
      : `• ${packageName(c.package)} overwrites files of ${c.modName} (${(c.files ?? []).join(", ")}): ${c.modName} will be uninstalled.`,
  );
  return `${lines.join("\n")}\n\nMods that depend on the uninstalled ones will be disabled.`;
}
