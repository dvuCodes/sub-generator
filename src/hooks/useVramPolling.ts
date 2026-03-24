import { useEffect, useRef, useState } from "react";
import type { VramInfo, VramInfoCommand } from "../lib/types";

interface UseVramPollingOptions {
  enabled: boolean;
  sendCommand: (cmd: VramInfoCommand) => Promise<void>;
  intervalMs?: number;
}

export function useVramPolling({
  enabled,
  sendCommand,
  intervalMs = 3000,
}: UseVramPollingOptions) {
  const [vram, setVram] = useState<VramInfo | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!enabled) {
      setVram(null);
      return;
    }

    sendCommand({ command: "vram_info" }).catch(console.error);

    intervalRef.current = setInterval(() => {
      sendCommand({ command: "vram_info" }).catch(console.error);
    }, intervalMs);

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, [enabled, sendCommand, intervalMs]);

  const handleVramResponse = (vramData: VramInfo | null) => {
    setVram(vramData);
  };

  return { vram, handleVramResponse };
}
