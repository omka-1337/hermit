import NexusBrowser from "../browse/NexusBrowser";
import { useProfile } from "./ProfileContext";

// nexusMod reads the id on Nexus Mods of a mod installed from there, from
// the address of its page.
const nexusMod = /nexusmods\.com\/[^/]+\/mods\/(\d+)/;

// NexusBrowseTab browses Nexus Mods for mods to download into the profile.
export default function NexusBrowseTab() {
  const { game, profile } = useProfile();
  const installed = new Set<number>();
  for (const m of profile.mods ?? []) {
    const id = m.source?.type === "nexus" && nexusMod.exec(m.source.url ?? "")?.[1];
    if (id) installed.add(Number(id));
  }
  return <NexusBrowser game={game} installed={installed} />;
}
