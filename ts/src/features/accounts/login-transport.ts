import { createContext } from "react";

export type LoginSocket = {
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  send: (data: string) => void;
  close: () => void;
};

export type LoginSocketFactory = (url: URL) => LoginSocket;

// Embeddings can supply their login channel without replacing the browser's
// WebSocket globally. Without a provider, LoginPanel connects to the Manager.
export const LoginTransportContext = createContext<
  LoginSocketFactory | undefined
>(undefined);
