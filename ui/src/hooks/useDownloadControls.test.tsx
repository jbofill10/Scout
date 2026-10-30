import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../contexts/ToastContext";
import { toSelection, useDownloadControls, useReleases, type ReleaseCandidate } from "./useDownloadControls";

vi.mock("../lib/logger", () => ({ logError: vi.fn() }));

const jsonResponse = (body: unknown, init?: ResponseInit) =>
  new Response(JSON.stringify(body), { status: 200, ...init });

function setup() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const invalidate = vi.spyOn(queryClient, "invalidateQueries");
  const toast = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <ToastContext.Provider value={toast}>{children}</ToastContext.Provider>
    </QueryClientProvider>
  );
  return { wrapper, toast, invalidate };
}

const candidate = (over: Partial<ReleaseCandidate> = {}): ReleaseCandidate => ({
  title: "Some.Show.S01E05.1080p.WEB.x264-GRP",
  indexer: "1337x",
  indexer_id: 2,
  guid: "magnet:?xt=urn:btih:abc",
  info_hash: "abc",
  size: 1_500_000_000,
  seeders: 40,
  leechers: 3,
  publish_date: "2026-06-01T00:00:00Z",
  quality: "1080p",
  accepted: true,
  score: 2.1,
  previously_failed: false,
  query: "Some Show S01E05",
  ...over,
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("toSelection", () => {
  it("carries the identifiers qBittorrent needs and fills missing ones with empty strings", () => {
    expect(toSelection(candidate({ download_url: undefined, info_hash: undefined }))).toEqual({
      guid: "magnet:?xt=urn:btih:abc",
      download_url: "",
      info_hash: "",
      title: "Some.Show.S01E05.1080p.WEB.x264-GRP",
      indexer: "1337x",
      indexer_id: 2,
      size: 1_500_000_000,
      seeders: 40,
      leechers: 3,
    });
  });
});

describe("useDownloadControls", () => {
  it("cancel posts to the item's cancel endpoint, toasts and refreshes the views", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ rows_cancelled: 1 }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast, invalidate } = setup();
    const { result } = renderHook(() => useDownloadControls(), { wrapper });

    act(() => result.current.cancel({ itemId: "55", title: "Some Show S01E05" }));

    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith("/api/downloads/55/cancel", expect.objectContaining({ method: "POST" }));
    expect(toast).toHaveBeenCalledWith("Cancelled Some Show S01E05", { severity: "info" });
    const keys = invalidate.mock.calls.map(([opts]) => (opts as { queryKey: unknown[] }).queryKey);
    expect(keys).toEqual(expect.arrayContaining([["activity"], ["notifications"], ["schedule"]]));
  });

  it("reports a pending cancel for the item being cancelled only", async () => {
    let resolve: (value: Response) => void = () => {};
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((r) => (resolve = r))));
    const { wrapper } = setup();
    const { result } = renderHook(() => useDownloadControls(), { wrapper });

    act(() => result.current.cancel({ itemId: "55", title: "x" }));

    await waitFor(() => expect(result.current.isCancelling("55")).toBe(true));
    expect(result.current.isCancelling("56")).toBe(false);
    expect(result.current.isRetrying("55")).toBe(false);
    act(() => resolve(jsonResponse({})));
    await waitFor(() => expect(result.current.isCancelling("55")).toBe(false));
  });

  it("retry explains a 409 instead of a generic failure", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ error: "busy" }, { status: 409 })));
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useDownloadControls(), { wrapper });

    act(() => result.current.retryNow({ itemId: "55", title: "Some Show" }));

    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(toast.mock.calls[0][0]).toMatch(/being dispatched right now/);
    expect(toast.mock.calls[0][1]).toEqual({ severity: "error" });
  });

  it("retry success toasts with a link to the activity page", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ rearmed: 1 })));
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useDownloadControls(), { wrapper });

    act(() => result.current.retryNow({ itemId: "55", title: "Some Show" }));

    await waitFor(() => expect(toast).toHaveBeenCalledWith("Retrying Some Show now", { severity: "success", linkToActivity: true }));
  });

  it("grab posts the selection and words the toast by outcome", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ tvdb_id: "55", outcome: "downloading" }))
      .mockResolvedValueOnce(jsonResponse({ tvdb_id: "55", outcome: "failed", reason: "Torrent client error" }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useDownloadControls(), { wrapper });
    const release = toSelection(candidate());

    act(() => result.current.grab({ itemId: "55", title: "Some Show", release }));
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/downloads/55/releases");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual(release);
    expect(toast).toHaveBeenLastCalledWith(`Downloading ${release.title}`, { severity: "success", linkToActivity: true });

    act(() => result.current.grab({ itemId: "55", title: "Some Show", release }));
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(2));
    expect(toast).toHaveBeenLastCalledWith("Torrent client error", { severity: "warning" });
  });

  it("a 404 says there is nothing left to act on", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ error: "gone" }, { status: 404 })));
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useDownloadControls(), { wrapper });

    act(() => result.current.cancel({ itemId: "55", title: "x" }));

    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(toast.mock.calls[0][0]).toMatch(/Nothing left to act on/);
  });
});

describe("useReleases", () => {
  it("stays idle without an item and fetches the item's releases once given one", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ item_id: "55", media_title: "Some Show", queries: 4, candidates: [candidate()] }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper } = setup();

    const idle = renderHook(() => useReleases(null), { wrapper });
    expect(idle.result.current.fetchStatus).toBe("idle");
    expect(fetchMock).not.toHaveBeenCalled();

    const { result } = renderHook(() => useReleases("55"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock).toHaveBeenCalledWith("/api/downloads/55/releases");
    expect(result.current.data?.candidates).toHaveLength(1);
  });

  it("defaults a missing candidate list to empty", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ item_id: "55", queries: 0 })));
    const { wrapper } = setup();
    const { result } = renderHook(() => useReleases("55"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.candidates).toEqual([]);
  });
});
