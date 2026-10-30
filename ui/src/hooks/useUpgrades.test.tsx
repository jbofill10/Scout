import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../contexts/ToastContext";
import { describeAttempt, useRunUpgrades, useUpgradeCandidates } from "./useUpgrades";

vi.mock("../lib/logger", () => ({ logError: vi.fn() }));

const jsonResponse = (body: unknown, init?: ResponseInit) => new Response(JSON.stringify(body), { status: 200, ...init });

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

describe("upgrades", () => {
  it("describes items", () => {
    expect(describeAttempt({ kind: "episode", title: "Some Show", season: 1, episode: 5 })).toBe("Some Show S01E05");
    expect(describeAttempt({ kind: "movie", title: "A Movie", year: 1994 })).toBe("A Movie (1994)");
    expect(describeAttempt({ kind: "movie", title: "A Movie" })).toBe("A Movie");
  });

  it("loads candidates", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ enabled: false, cutoff: "1080p", batch: 3, total: 1, deferred: 0, candidates: [{ item_key: "1", kind: "episode", title: "X", current_quality: "720p", target_quality: "1080p", files: [] }] })));
    const { wrapper } = setup();
    const { result } = renderHook(() => useUpgradeCandidates(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.total).toBe(1);
  });

  it("run words the toast by what started and explains a 412", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ started: 2, items: ["A S01E01 (720p)", "B (480p)"] }, { status: 202 }))
      .mockResolvedValueOnce(jsonResponse({ error: "Upgrades are turned off in Settings" }, { status: 412 }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useRunUpgrades(), { wrapper });

    act(() => result.current.mutate());
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(toast).toHaveBeenLastCalledWith("Looking for better copies of 2 items: A S01E01 (720p), B (480p)", { severity: "success", linkToActivity: true });

    act(() => result.current.mutate());
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(2));
    expect(toast).toHaveBeenLastCalledWith("Upgrades are turned off in Settings", { severity: "error" });
  });
});
