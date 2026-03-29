import { useCallback, useEffect, useRef, useState } from "react";
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
  const awaitingResponseRef = useRef(false);

  const requestVram = useCallback(() => {
    if (awaitingResponseRef.current) {
      return;
    }

    awaitingResponseRef.current = true;
    sendCommand({ command: "vram_info" }).catch((error) => {
      awaitingResponseRef.current = false;
      console.error(error);
    });
  }, [sendCommand]);

  useEffect(() => {
    if (!enabled) {
      awaitingResponseRef.current = false;
      return;
    }

    requestVram();

    intervalRef.current = setInterval(() => {
      requestVram();
    }, intervalMs);

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
      awaitingResponseRef.current = false;
    };
  }, [enabled, intervalMs, requestVram]);

  const handleVramResponse = useCallback((vramData: VramInfo | null) => {
    awaitingResponseRef.current = false;
    setVram(vramData);
  }, []);

  return { vram: enabled ? vram : null, handleVramResponse };
}
