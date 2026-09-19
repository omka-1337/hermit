# Game compatibility

Any game on Thunderstore that uses BepInEx should work with Hermit: mods are installed into the profile and reach the
game through symlinks, so nothing is written into the game folder. The list below is not what is supported, it is what
has actually been tried.

| Game | Runtime | Status | What was done |
| --- | --- | --- | --- |
| Lethal Company | Proton | Works | Mods installed, enabled and disabled, launched through Steam |
| PEAK | Proton | Works | Same, plus installing a modpack as a profile of its own |
| Content Warning | Proton | Partly checked | Browsing and installing; not launched |
| R.E.P.O. | Proton | Partly checked | Browsing; not installed or launched |
| Valheim | Proton | Partly checked | Browsing; not installed or launched |
| Native Linux builds | native | Untested | Loading BepInEx through `run_bepinex.sh` is implemented but never run against a real game |
| IL2CPP games (BepInEx 6) | either | Untested | The backend is detected and the right pack is installed, but no IL2CPP game has been run |

Status means:

- **Works** — mods were installed and the game started with them loaded.
- **Partly checked** — some of it was exercised, the column on the right says how far.
- **Untested** — the code path exists, nobody has run it.

## Adding to this list

Reports of games that work are as useful as reports of games that do not. Open a
[game report](https://github.com/omka-1337/hermit/issues/new?template=game_report.yml) with the game, how it
runs (Proton or a native build), what you installed and what happened. Anything BepInEx wrote after a failed session
helps too — Hermit shows it in the profile after the game exits.
