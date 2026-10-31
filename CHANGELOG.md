# Changelog

## 0.3.0 — 2026-10-31

Hermit is no longer only for BepInEx: games whose mods are files laid out as the game's own get a load order of mods,
as in Mod Organizer 2, with the game folder still untouched outside a session.

### Added

- Creation Engine games: Skyrim (Special Edition, VR, Legendary), Fallout 4 (and VR) and Starfield. A profile's mods
  are linked into Data for the length of a session, in the profile's order, and what the game writes there meanwhile
  is kept in the profile's overwrite folder. Each profile has its own plugin list and INI files, and the game starts
  through its script extender when the profile has one.
- Import from Mod Organizer 2: mods, their order, separators, the plugin list, INI files and the overwrite folder,
  into an empty profile or as a new one.
- The load order of mods: drag mods into place, with the list scrolling near its edges; group them under separators
  that fold; click a mod to see in green the mods it wins over and in red those that win over it. Mods are listed in
  columns — Active, Name, Version, Source, Author — and removed from a right-click menu. F5 takes in mod folders
  changed by hand.
- Mod archives — zip, 7z and rar — unpacked without tools from the system, FOMOD installers included, each in a window
  of its own. Several archives install at once through a queue that keeps unpacking while an installer is open. An
  archive of a mod the profile has already replaces it, merges with it or is installed under another name.
- `hermit install` installs mod archives from the command line, FOMOD options included.
- Nexus Mods: log in with a personal API key, and "Mod Manager Download" opens in Hermit and installs into the game's
  active profile, keeping the archive in the profile's downloads folder. A Downloads page follows the queue, and a
  warning says what is missing until it is set up.
- Tools that come with mods, such as BodySlide or Pandora, run in the game's Proton prefix with the profile in place.
- A Logs page with what SKSE and its plugins wrote.
- Cyberpunk 2077 (REDengine 4) and Batman: Arkham Knight (Unreal Engine 3), managed the same way. Their mods go into
  the game folder and may take the place of its files for the session; DLLs beside the game's executable, such as
  Cyber Engine Tweaks' and RED4ext's, are loaded through Wine's native overrides. Not yet tried with real mods.
- The engine a game is built on is shown beside it: the Unity version, Creation Engine, REDengine or Unreal Engine.
- Choose the file a game starts from, including one a mod brings, also when Steam starts the game.
- Keep a profile in a folder of its own, such as one on the disk the game is on.
- Open a profile's folder in the file manager.
- Choose the folder of the game a mod's files go into when they come without one.
- After a session, see which mods loaded and which are missing, the errors, and where loading stopped.
- Modpacks keep installing while you move on to other pages, can be cancelled, and old ones come with a warning.
- Add Hermit to Steam as a non-Steam game from Settings, for Game Mode on a Steam Deck.
- Games set up for PortProton start through it, also when PortProton is a Flatpak.
- Install BepInEx from its GitHub releases. A profile without a loader says so and offers the build the game needs —
  BepInEx 5 for Mono, 6 for IL2CPP, Windows or Linux files — for games that have no Thunderstore page or are modded
  from elsewhere, where nothing pulls the pack in as a dependency.
- Settings show the size of the download cache and can clear it.
- Duplicate a profile: the same mods and configs in a profile of its own.

### Changed

- The desktop layout is a sidebar: the game picked from a list at the top, then its profile, then the profile's
  pages, with Browse, Downloads and Settings at the bottom. Hermit and File menus sit in the title bar, Play in the
  top right corner, and a status bar along the bottom counts mods and follows downloads and installs. The game and
  profile open last are open again when Hermit starts.
- Profiles are cards: a click opens the profile, a right click (or the ⋯ button, for touch and the controller) opens
  a menu with Set active, Rename, Duplicate and Remove, and the active one is marked by a strip along its left edge.
- The hermit crab is Hermit's icon, and its teal the accent colour.

### Fixed

- Updating mods shows what is happening: the mod being updated has its download percentage, the ones already through
  are ticked off one by one instead of all at the end, and the rest say they are waiting rather than just going grey.
- Archives made on Windows with backslashes in their folder names install.
- Under Wayland the window has Hermit's icon rather than a generic one.

## 0.2.0 — 2026-09-18

### Added

- Steam Deck layout: a console-style bar of games along the top, switched with LB/RB, full screen and without a title
  bar.
- Controller navigation: the d-pad moves the focus, A selects, B steps back and finally leaves Hermit. Dialogs and
  confirmations answer to the controller as well, and LT/RT switch the tabs of a profile.
- Card view in the mod browser, with a cards/list switch that is remembered between runs.
- Update progress: a spinner on the mod being updated, a green tick when it is done, and a visible result of a check.
- Update check for Hermit itself: About tells when a newer release exists and links to it.
- Licensed under the GNU GPL v3 or later, with the licenses of every dependency listed in
  [THIRD-PARTY.md](THIRD-PARTY.md).
- Screenshots and install instructions in the README.

### Changed

- The running game and the Steam launch options are polled, so their state no longer sticks until the page is
  reopened.
- Adding a game in the Steam Deck layout is a page of its own instead of a dialog.

### Fixed

- Flicker when switching between games: icons are painted from cache and the page keeps its shape while it loads.
- Entering a game page focused the rename pencil instead of the active profile.
- The selection ring in the game bar was clipped at the top and bottom.
- Steam's `localconfig.vdf` was reparsed on every poll; it is now cached until the file changes.

## 0.1.0 — 2026-09-17

First release, as an AppImage.

### Added

- Games found in Steam libraries, with the runtime (Proton or native) and the Unity backend detected automatically.
- Profiles per game: mods live in the profile and reach the game through symlinks, so the game folder stays untouched.
- Thunderstore browser with dependency resolution, conflict detection, updates and modpacks, where a modpack becomes a
  profile of its own.
- Mods from local `.zip` and `.dll` files and from GitHub releases, with BepInEx plugin metadata read out of the
  assemblies.
- Enabling and disabling mods, with a mod force-disabled while its dependencies are missing.
- Config file editor for installed mods.
- Launching from Hermit or straight from Steam through launch options, so mods load without the manager running.
- Profile export and import compatible with r2modman: `.r2z` files and Thunderstore profile codes.
- A custom window frame instead of the desktop's own decorations.
