import { useEffect, useState } from "react";
import { CreationService, errorMessage, Game, ProfileOptions } from "../../api";
import { ErrorText, Modal, Spinner, Toggle } from "../ui";

type Props = {
  game: Game;
  profileId: string;
  profileName: string;
  onClose: () => void;
};

// ProfileOptionsModal is the settings of a Creation Engine profile: whether
// it plays with INI files and saves of its own or with the game's, as MO2's
// profiles choose.
export default function ProfileOptionsModal({ game, profileId, profileName, onClose }: Props) {
  const [options, setOptions] = useState<ProfileOptions | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    CreationService.Options(game.id, profileId)
      .then(setOptions)
      .catch((err) => setError(errorMessage(err)));
  }, [game.id, profileId]);

  const change = async (action: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await action();
      setOptions(await CreationService.Options(game.id, profileId));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={`${profileName} settings`} onClose={onClose}>
      {options === null ? (
        error ? (
          <ErrorText>{error}</ErrorText>
        ) : (
          <div className="flex justify-center py-6">
            <Spinner />
          </div>
        )
      ) : (
        <div className="flex flex-col gap-5">
          <Option
            title="INI files of the profile's own"
            checked={options.profileInis}
            disabled={busy}
            onChange={(on) => change(() => CreationService.SetProfileINIs(game.id, profileId, on))}
          >
            The game plays with the profile's copies of its INI files, and what is changed in its settings stays in the
            profile. Off, it plays with the game's own, the ones every profile shares; the profile's are kept for when
            this is on again. The first time, the profile starts from copies of the game's.
          </Option>
          <Option
            title="Saves of the profile's own"
            checked={options.profileSaves}
            disabled={busy}
            onChange={(on) => change(() => CreationService.SetProfileSaves(game.id, profileId, on))}
          >
            The game saves into the profile and sees only the profile's saves, so a playthrough stays with the mods it
            was played with. Off, the game uses its own Saves folder, the one every profile shares. The profile starts
            with no saves; the game's stay where they are.
          </Option>
          <ErrorText>{error}</ErrorText>
        </div>
      )}
    </Modal>
  );
}

function Option({
  title,
  checked,
  disabled,
  onChange,
  children,
}: {
  title: string;
  checked: boolean;
  disabled: boolean;
  onChange: (on: boolean) => void;
  children: string;
}) {
  return (
    <div className="flex items-start gap-4">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-xs text-zinc-400">{children}</span>
      </div>
      <Toggle checked={checked} disabled={disabled} title={title} onChange={onChange} />
    </div>
  );
}
