import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../contexts/ToastContext";
import { unrequestedCount, useFillMissing, useMissingEpisodes, type MissingShow } from "./useMissing";

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

const show: MissingShow = {
  tvdbId: "900",
  title: "Some Show",
  thumb: "/t",
  episodes: [
    { tvdbId: "1", seasonNumber: 1, episodeNumber: 1, name: "One", aired: "2026-01-01", downloaded: false, requested: false },
    { tvdbId: "2", seasonNumber: 1, episodeNumber: 2, name: "Two", aired: "2026-01-08", downloaded: false, requested: true },
  ],
};

describe("useMissing", () => {
  it("counts what still needs requesting", () => {
    expect(unrequestedCount(show)).toBe(1);
  });

  it("loads the list and defaults a non-array to empty", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse([show])));
    const { wrapper } = setup();
    const { result } = renderHook(() => useMissingEpisodes(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0].episodes).toHaveLength(2);
  });

  it("posts the fill and words the toast from the summary", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ media_title: "Some Show", category: "series", queued_now: 1, scheduled: 0, skipped: 1 }, { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useFillMissing(), { wrapper });

    act(() => result.current.mutate({ tvdbId: "900", title: "Some Show" }));

    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith("/api/library/shows/900/fill-missing", { method: "POST" });
    expect(toast).toHaveBeenCalledWith("Some Show: searching for 1 episode now", { severity: "success", linkToActivity: true });
  });

  it("says so when nothing was left to request", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ media_title: "Some Show", category: "series", queued_now: 0, scheduled: 0, skipped: 2 }, { status: 202 })));
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useFillMissing(), { wrapper });
    act(() => result.current.mutate({ tvdbId: "900", title: "Some Show" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("Some Show: already tracked — nothing new to download", { severity: "info", linkToActivity: false }));
  });
});
