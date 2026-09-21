import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { errorMessage, InstallService, isCancelled, Progress } from "./api";

// Installing a modpack takes minutes and keeps going when its page is closed
// or another modpack is opened. Its state lives here rather than in the
// button, so coming back shows the install in progress (with Cancel) instead
// of offering to start a second one.

export type ModpackInstall =
  | { status: "running"; progress: Progress | null }
  | { status: "failed"; error: string };

// Settled is told about every install that ends. profileId is set only when
// it succeeded; a cancel that came too late may still have made a profile, so
// listeners that list profiles should look again either way.
type Settled = { gameId: string; packageId: string; profileId?: string };

const installs = new Map<string, ModpackInstall>();
const cancels = new Map<string, () => void>();
const listeners = new Set<() => void>();
const settledListeners = new Set<(settled: Settled) => void>();

const key = (gameId: string, packageId: string) => `${gameId}/${packageId}`;

function notify() {
  for (const listener of listeners) listener();
}

// Progress events name the game and the package being installed, which is
// enough to find the install they belong to.
Events.On("install:progress", (ev) => {
  const p: Progress = ev.data;
  const k = key(p.gameId, p.target);
  if (installs.get(k)?.status === "running") {
    installs.set(k, { status: "running", progress: p });
    notify();
  }
});

// startModpackInstall creates a profile from a modpack unless the same one is
// already being installed.
export function startModpackInstall(gameId: string, namespace: string, name: string, version: string, profileName: string) {
  const packageId = `${namespace}-${name}`;
  const k = key(gameId, packageId);
  if (installs.get(k)?.status === "running") return;

  const call = InstallService.InstallModpack(gameId, namespace, name, version, profileName);
  installs.set(k, { status: "running", progress: null });
  cancels.set(k, () => call.cancel());
  notify();

  let profileId: string | undefined;
  call
    .then((profile) => {
      installs.delete(k);
      profileId = profile.id;
    })
    .catch((err) => {
      // A cancelled install removed its half-made profile; nothing to report.
      if (isCancelled(err)) installs.delete(k);
      else installs.set(k, { status: "failed", error: errorMessage(err) });
    })
    .finally(() => {
      cancels.delete(k);
      notify();
      for (const listener of settledListeners) listener({ gameId, packageId, profileId });
    });
}

export function cancelModpackInstall(gameId: string, packageId: string) {
  cancels.get(key(gameId, packageId))?.();
}

// useModpackInstall follows the install of one modpack for one game.
export function useModpackInstall(gameId: string, packageId: string): ModpackInstall | undefined {
  const k = key(gameId, packageId);
  const [state, setState] = useState(() => installs.get(k));
  useEffect(() => {
    const update = () => setState(installs.get(k));
    listeners.add(update);
    update();
    return () => void listeners.delete(update);
  }, [k]);
  return state;
}

// onModpackSettled is called for every modpack install that ends, whether or
// not its page is still open, so the game's list of profiles can catch up.
export function onModpackSettled(listener: (settled: Settled) => void): () => void {
  settledListeners.add(listener);
  return () => void settledListeners.delete(listener);
}
