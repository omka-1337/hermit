import { CancelError } from "@wailsio/runtime";
import { ask } from "./confirmhost";

export { Backend, Library, Runtime } from "../bindings/hermit/internal/library";
export type { Game, GameCandidate, Mod, Profile } from "../bindings/hermit/internal/library";
export { Store as SettingsStore } from "../bindings/hermit/internal/settings";
export { UIMode, BrowseView } from "../bindings/hermit/internal/settings";
export type { Settings } from "../bindings/hermit/internal/settings";
export {
  BrowseService,
  CacheService,
  ConfigService,
  FolderService,
  IconService,
  InfoService,
  InstallService,
  LaunchService,
  ShareService,
  SteamService,
} from "../bindings/hermit/internal/app";
export type { ImportProgress, ImportResult, Preview } from "../bindings/hermit/internal/profileshare";
export type { ConfigContent } from "../bindings/hermit/internal/app";
export type { Change, Entry, FileInfo as ConfigFileInfo } from "../bindings/hermit/internal/configs";
export type { LaunchInfo } from "../bindings/hermit/internal/app";
export { ConflictReason, Stage } from "../bindings/hermit/internal/modinstall";
export type {
  Conflict,
  InstallPlan,
  LocalPackage,
  Progress,
  UpdateResult,
} from "../bindings/hermit/internal/modinstall";
export type { GitHubRepo } from "../bindings/hermit/internal/app";
export type { Asset as GitHubAsset, Release as GitHubRelease } from "../bindings/hermit/internal/github";
export { IssueKind } from "../bindings/hermit/internal/launch";
export type { Issue, Report } from "../bindings/hermit/internal/launch";
export { Ordering } from "../bindings/hermit/internal/thunderstore";
export type {
  Community,
  Filters,
  PackageDetail,
  PackageList,
  PackageSummary,
} from "../bindings/hermit/internal/thunderstore";
export type { AppInfo, BepInExBuild, LoaderStatus, SteamSetup, Update } from "../bindings/hermit/internal/app";

// isCancelled reports whether err comes from cancelling a CancellablePromise.
export function isCancelled(err: unknown): boolean {
  return err instanceof CancelError;
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// confirmDanger asks before an irreversible action; resolves true if confirmed.
export function confirmDanger(title: string, message: string, action: string): Promise<boolean> {
  return ask({ title, message, action, danger: true });
}

// confirm asks a yes/no question; resolves true if the action button was chosen.
export function confirm(title: string, message: string, action: string): Promise<boolean> {
  return ask({ title, message, action });
}
