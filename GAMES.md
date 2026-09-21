# Game compatibility

Any game on Thunderstore that uses BepInEx should work with Hermit: mods are installed into the profile and reach the
game through symlinks, so nothing is written into the game folder. The list below is not what is supported, it is what
has actually been tried.

| Game | Runtime | Status | What was done |
| --- | --- | --- | --- |
| Lethal Company | Linux (Proton) | Works | Mods installed and launched |
| PEAK | Linux (Proton) | Works | Mods installed and launched |
| Easy Delivery Co. | Linux (Proton) | Works | Mods installed and launched |
| Content Warning | Linux (Proton) | Works | Mods installed and launched |
| R.E.P.O. | Linux (Proton) | Works | Mods installed and launched |
| Valheim | Linux (**Native**/Proton) | Works | Mods installed and launched |
| MiSide | Linux (Proton) | Works | Mods installed and launched |
| Native Linux builds | native | Untested | Loading BepInEx through `run_bepinex.sh` is implemented but never run against a real game |
| Other IL2CPP games (BepInEx 6) | either | Partly checked | Works on MiSide; other IL2CPP games have not been tried |

Status means:

- **Works** — mods were installed and the game started with them loaded.
- **Partly checked** — some of it was exercised, the column on the right says how far.
- **Untested** — the code path exists, nobody has run it.

## Old modpacks

A modpack pins the exact version of every mod in it, the versions it was put together with. That is what makes it
reproducible, and also what makes it age: once the game updates, mods from before the update may call code that no
longer exists. This is not specific to any game or to Hermit — r2modman installs the same versions — but it is the
most common reason a modpack "does not work". It shows up in two ways:

- the game hangs or goes black while mods are loading, and BepInEx never finishes;
- the game starts, but the log fills with errors such as `MissingMethodException`.

Both were seen in practice: a Content Warning modpack from 2024 hung after the game moved to Unity 6, and a R.E.P.O.
modpack from May 2025 logged over a thousand errors.

Hermit warns before installing a modpack whose last update is more than half a year old. If one does not start,
**Update all** in its profile is the first thing to try: it moves every mod to its latest version. After each launch the
profile shows how many mods BepInEx loaded, which it never got to, where loading stopped and how many errors the log
has — enough to tell an outdated modpack from a broken setup.

## Adding to this list

Reports of games that work are as useful as reports of games that do not. Open a
[game report](https://github.com/omka-1337/hermit/issues/new?template=game_report.yml) with the game, how it
runs (Proton or a native build), what you installed and what happened. Anything BepInEx wrote after a failed session
helps too — Hermit shows it in the profile after the game exits.
