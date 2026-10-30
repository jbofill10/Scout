import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../contexts/ToastContext";
import { unhealthyCount, useIndexerHealth, useRotateRelay, type IndexerHealthSnapshot } from "./useIndexerHealth";

vi.mock("../lib/logger", () => ({ logError: vi.fn() }));

const jsonResponse = (body: unknown, init?: ResponseInit) => new Response(JSON.stringify(body), { status: 200, ...init });

const snapshot: IndexerHealthSnapshot = {
  indexers: [
    { id: 1, name: "Nyaa.si", enabled: true, healthy: true, ip_blocked: false },
    { id: 2, name: "1337x", enabled: true, healthy: false, ip_blocked: true, failure: "Disabled until 14:00" },
    { id: 3, name: "Old", enabled: false, healthy: false, ip_blocked: false },
  ],
  relay: { configured: true, connected: true, relay: "ca-mtr-wg-004", ip: "188.241.176.194", location: "mtr, ca" },
  events: [],
  auto_rotate: true,
};

function setup() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const toast = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <ToastContext.Provider value={toast}>{children}</ToastContext.Provider>
    </QueryClientProvider>
  );
  return { wrapper, toast };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("useIndexerHealth", () => {
  it("counts only enabled indexers that are disabled by Prowlarr", () => {
    expect(unhealthyCount(snapshot)).toBe(1);
    expect(unhealthyCount(undefined)).toBe(0);
  });

  it("loads the snapshot", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(snapshot)));
    const { wrapper } = setup();
    const { result } = renderHook(() => useIndexerHealth(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.relay.relay).toBe("ca-mtr-wg-004");
    expect(fetch).toHaveBeenCalledWith("/api/indexers/health", undefined);
  });

  it("rotation toasts the event and explains a refusal", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ at: "", kind: "rotation", message: "Relay changed from a to b", ok: true }))
      .mockResolvedValueOnce(jsonResponse({ error: "relay agent refused to rotate: rotated 5s ago" }, { status: 409 }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useRotateRelay(), { wrapper });

    act(() => result.current.mutate());
    await waitFor(() => expect(toast).toHaveBeenCalledWith("Relay changed from a to b", { severity: "success" }));
    expect(fetchMock.mock.calls[0][0]).toBe("/api/indexers/rotate");

    act(() => result.current.mutate());
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(2));
    expect(toast).toHaveBeenLastCalledWith("relay agent refused to rotate: rotated 5s ago", { severity: "error" });
  });
});
