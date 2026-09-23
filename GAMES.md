# Compatibility

Hermit works wherever BepInEx works. It does not load anything into a game itself: it installs the BepInEx pack and
the mods into a profile and links that profile into the game for the length of a session. Whether a given game can be
modded at all is BepInEx's question, and BepInEx answers it in its own
[platform compatibility chart](https://github.com/BepInEx/BepInEx#platform-compatibility-chart) — Unity Mono, Unity
IL2CPP and .NET Framework games, per platform.

BepInEx itself is a package like any other. Games with a Thunderstore community have their own pack there, and every
mod on Thunderstore lists it as a dependency, so installing the first mod brings it into the profile along the way.
Not every game has such a page, and mods from Nexus Mods or an archive on disk carry no dependencies at all — there
Hermit installs BepInEx from its own [GitHub releases](https://github.com/BepInEx/BepInEx/releases). A profile
without a loader says so and offers to install the build the game needs; until one is there Hermit links nothing
into the game and it starts vanilla.

What Hermit does around it:

- it reads the scripting backend from the game's files, which is what picks the build: Mono games get BepInEx 5,
  IL2CPP games BepInEx 6, and how the game runs picks Windows or Linux files;
- for games that run through Proton it sets the `winhttp` override for the session instead of writing it into the
  prefix;
- for native Linux builds it starts the game through the pack's own `run_bepinex.sh`. That path is implemented but
  has not been run against a real game yet — everything tried so far ran through Proton.

Used in practice on Lethal Company, PEAK, Content Warning (Unity 6000.0.67), R.E.P.O. (Unity 2022.3.67), Easy
Delivery Co. and MiSide (Unity 2021.3.35, IL2CPP): installing mods, enabling and disabling them, modpacks, and
launching through Steam or, outside Steam, through PortProton.

## Other engines

Games that do not use BepInEx — Cyberpunk 2077, The Witcher 3, Unreal Engine titles, Bethesda's games — each have
their own mod layout, their own loaders and their own rules about load order. They are not supported, and when they
are, they will need their own pages: nothing about them can be said in one line the way it can for BepInEx.

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
**Update all** in its profile is the first thing to try: it moves every mod to its latest version. After each launch
the profile shows how many mods BepInEx loaded, which it never got to, where loading stopped and how many errors the
log has — enough to tell an outdated modpack from a broken setup.

## Reporting a game

A game BepInEx supports but Hermit does not handle is a bug worth hearing about — an unusual layout, a pack Hermit
installs wrongly, or something in the launch path. Open a
[game report](https://github.com/omka-1337/hermit/issues/new?template=game_report.yml) with the game, how it runs
(Proton or a native build), what you installed and what happened. Anything BepInEx wrote after a failed session helps
too — Hermit shows it in the profile after the game exits.
