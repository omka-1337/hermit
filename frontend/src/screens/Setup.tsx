import { useState } from "react";
import { Game } from "../api";
import GamePicker from "../components/GamePicker";
import { Button } from "../components/ui";

type Props = {
  onFinish: (game?: Game) => void;
};

export default function Setup({ onFinish }: Props) {
  const [lastAdded, setLastAdded] = useState<Game | null>(null);

  return (
    <div className="flex h-full items-center justify-center p-6">
      <div className="w-full max-w-2xl rounded-lg border border-zinc-800 bg-zinc-900 p-8">
        <h1 className="mb-2 text-2xl font-semibold">Add a game</h1>
        <p className="mb-6 text-sm text-zinc-400">
          Add the games you want to mod. You can also do this later.
        </p>
        <GamePicker
          onAdded={setLastAdded}
          actions={
            lastAdded ? (
              <Button variant="primary" onClick={() => onFinish(lastAdded)}>
                Continue
              </Button>
            ) : (
              <Button variant="ghost" onClick={() => onFinish()}>
                Skip
              </Button>
            )
          }
        />
      </div>
    </div>
  );
}
