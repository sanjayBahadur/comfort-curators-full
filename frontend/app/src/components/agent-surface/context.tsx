import { createContext, useCallback, useContext, useEffect, useRef, type ReactNode } from "react";
import type { AgentAction, AgentSurfaceEntry, AgentSurfaceRegistry } from "./types";
import { PaymentBoundaryContext } from "../superhost/PaymentBoundary";

type AgentSurfaceContextValue = {
  registry: AgentSurfaceRegistry;
  register: (entry: AgentSurfaceEntry) => void;
  unregister: (id: string) => void;
  clearAll: () => void;
  restoreAll: () => void;
};

const AgentSurfaceContext = createContext<AgentSurfaceContextValue | null>(null);

export function AgentSurfaceProvider({ children }: { children: ReactNode }) {
  const registryRef = useRef<AgentSurfaceRegistry>(new Map());
  const knownEntriesRef = useRef<AgentSurfaceRegistry>(new Map());

  const register = useCallback((entry: AgentSurfaceEntry) => {
    registryRef.current.set(entry.id, entry);
    knownEntriesRef.current.set(entry.id, entry);
  }, []);

  const unregister = useCallback((id: string) => {
    registryRef.current.delete(id);
    knownEntriesRef.current.delete(id);
  }, []);

  const clearAll = useCallback(() => {
    registryRef.current.clear();
  }, []);

  const restoreAll = useCallback(() => {
    registryRef.current.clear();
    for (const [id, entry] of knownEntriesRef.current) {
      if (entry.element.isConnected) registryRef.current.set(id, entry);
    }
  }, []);

  return (
    <AgentSurfaceContext.Provider value={{ registry: registryRef.current, register, unregister, clearAll, restoreAll }}>
      {children}
    </AgentSurfaceContext.Provider>
  );
}

export function useAgentSurfaceContext(): AgentSurfaceContextValue {
  const ctx = useContext(AgentSurfaceContext);
  if (!ctx) throw new Error("useAgentSurfaceContext must be used within AgentSurfaceProvider");
  return ctx;
}

export function useAgentSurface(
  id: string,
  actions: AgentAction[],
  label: string,
  onOpenPanel?: () => void,
) {
  const { register, unregister } = useAgentSurfaceContext();
  const { isInside: insidePaymentBoundary } = useContext(PaymentBoundaryContext);
  const registeredRef = useRef(false);
  const actionsRef = useRef(actions);
  actionsRef.current = actions;
  const labelRef = useRef(label);
  labelRef.current = label;
  const onOpenPanelRef = useRef(onOpenPanel);
  onOpenPanelRef.current = onOpenPanel;

  useEffect(() => {
    return () => {
      if (registeredRef.current) {
        unregister(id);
        registeredRef.current = false;
      }
    };
  }, [id, unregister]);

  const ref = useCallback(
    (node: HTMLElement | null) => {
      if (node) {
        if (insidePaymentBoundary) return;
        const currentActions = actionsRef.current;
        const currentLabel = labelRef.current;
        const currentOnOpenPanel = onOpenPanelRef.current;
        node.setAttribute("data-agent", id);
        node.setAttribute("data-agent-actions", currentActions.join(" "));
        node.setAttribute("data-agent-label", currentLabel);
        register({
          id,
          element: node,
          actions: new Set(currentActions),
          label: currentLabel,
          onOpenPanel: currentOnOpenPanel,
        });
        registeredRef.current = true;
      }
    },
    [id, register, insidePaymentBoundary],
  );

  return { ref };
}
