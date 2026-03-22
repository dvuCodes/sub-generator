import { useState, useCallback, useEffect, useRef } from "react";
import { invoke } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import type { SidecarCommand, SidecarResponse } from "../lib/types";

interface SidecarState {
  connected: boolean;
  connecting: boolean;
}

export function useSidecar() {
  const [state, setState] = useState<SidecarState>({
    connected: false,
    connecting: false,
  });
  const [lastResponse, setLastResponse] = useState<SidecarResponse | null>(
    null
  );
  const listenersRef = useRef<UnlistenFn[]>([]);
  const responseHandlerRef = useRef<((response: SidecarResponse) => void) | null>(null);

  // Set up event listeners for sidecar output
  useEffect(() => {
    const setupListeners = async () => {
      const unlistenOutput = await listen<string>("sidecar-output", (event) => {
        try {
          const response = JSON.parse(event.payload) as SidecarResponse;
          setLastResponse(response);
          if (responseHandlerRef.current) {
            responseHandlerRef.current(response);
          }
        } catch {
          console.error("Failed to parse sidecar output:", event.payload);
        }
      });

      const unlistenError = await listen<string>("sidecar-error", (event) => {
        console.error("[sidecar]", event.payload);
      });

      const unlistenTerminated = await listen<number | null>(
        "sidecar-terminated",
        (event) => {
          console.warn("Sidecar terminated with code:", event.payload);
          setState({ connected: false, connecting: false });
        }
      );

      listenersRef.current = [
        unlistenOutput,
        unlistenError,
        unlistenTerminated,
      ];
    };

    setupListeners();

    return () => {
      listenersRef.current.forEach((unlisten) => unlisten());
    };
  }, []);

  const connect = useCallback(async () => {
    if (state.connected || state.connecting) return;

    setState({ connected: false, connecting: true });
    try {
      await invoke("spawn_sidecar");
      setState({ connected: true, connecting: false });
    } catch (err) {
      console.error("Failed to spawn sidecar:", err);
      setState({ connected: false, connecting: false });
      throw err;
    }
  }, [state.connected, state.connecting]);

  const disconnect = useCallback(async () => {
    try {
      await invoke("kill_sidecar");
    } catch (err) {
      console.error("Failed to kill sidecar:", err);
    }
    setState({ connected: false, connecting: false });
  }, []);

  const sendCommand = useCallback(
    async (command: SidecarCommand) => {
      if (!state.connected) {
        throw new Error("Sidecar is not connected");
      }
      const message = JSON.stringify(command);
      await invoke("send_to_sidecar", { message });
    },
    [state.connected]
  );

  const onResponse = useCallback(
    (handler: (response: SidecarResponse) => void) => {
      responseHandlerRef.current = handler;
    },
    []
  );

  return {
    connected: state.connected,
    connecting: state.connecting,
    lastResponse,
    connect,
    disconnect,
    sendCommand,
    onResponse,
  };
}
