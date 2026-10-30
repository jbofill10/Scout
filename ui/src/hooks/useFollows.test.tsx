import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../contexts/ToastContext";
import { describeCheck, useCheckFollows, useFollowShow, useUnfollowShow, type FollowCheckResult } from "./useFollows";

vi.mock("../lib/logger", () => ({ logError: vi.fn() }));

const jsonResponse = (body: unknown, init?: ResponseInit) => new Response(JSON.stringify(body), { status: 200, ...init });

const check = (over: Partial<FollowCheckResult> = {}): FollowCheckResult => ({
  tvdb_id: "900", title: "Some Show", listed: 10, new: 0, queued_now: 0, scheduled: 0, checked_at: "2026-06-15T12:00:00Z", ...over,
});

function setup() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const invalidate = vi.spyOn(queryClient, "invalidateQueries");
  const toast = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <ToastContext.Provider value={toast}>{children}</ToastContext.Provider>
    </QueryClientProvider>
  );
  return { wrapper, toast, invalidate };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("describeCheck", () => {
  it.each([
    [check(), "Nothing new to request"],
    [check({ new: 3, queued_now: 1, scheduled: 2 }), "searching for 1 now, 2 scheduled"],
    [check({ new: 2, scheduled: 2 }), "2 scheduled"],
    [check({ error: "tvdb down" }), "Check failed: tvdb down"],
  ])("%j", (result, expected) => {
    expect(describeCheck(result)).toBe(expected);
  });
});

describe("useFollowShow", () => {
  it("posts a series snapshot with the include flag and refreshes the views", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ show: { tvdb_id: "900", title: "Some Show" }, check: check({ new: 2, scheduled: 2 }) }, { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast, invalidate } = setup();
    const { result } = renderHook(() => useFollowShow(), { wrapper });

    act(() => result.current.mutate({ media: { id: "900", mediaName: "Some Show", image_url: "p" }, includeMissing: true }));

    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/follows");
    expect(JSON.parse(init.body)).toEqual({ media: { id: "900", mediaName: "Some Show", image_url: "p", type: "series" }, include_missing: true });
    expect(toast).toHaveBeenCalledWith("Following Some Show: 2 scheduled", { severity: "success", linkToActivity: true });
    const keys = invalidate.mock.calls.map(([o]) => (o as { queryKey: unknown[] }).queryKey);
    expect(keys).toEqual(expect.arrayContaining([["follows"], ["activity"], ["schedule"]]));
  });

  it("reports the server's error message", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ error: "Only shows can be followed" }, { status: 400 })));
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useFollowShow(), { wrapper });
    act(() => result.current.mutate({ media: { id: "1", mediaName: "A Movie" }, includeMissing: false }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("Only shows can be followed", { severity: "error" }));
  });
});

describe("useUnfollowShow and useCheckFollows", () => {
  it("deletes the follow", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useUnfollowShow(), { wrapper });
    act(() => result.current.mutate({ tvdbId: "900", title: "Some Show" }));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("Unfollowed Some Show", { severity: "info" }));
    expect(fetchMock).toHaveBeenCalledWith("/api/follows/900", { method: "DELETE" });
  });

  it("words a check-all report and a single check differently", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ checked: 3, added: 1, results: [] }))
      .mockResolvedValueOnce(jsonResponse(check({ new: 1, queued_now: 1 })));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useCheckFollows(), { wrapper });

    act(() => result.current.mutate({ tvdbId: null }));
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0][0]).toBe("/api/follows/check");
    expect(toast).toHaveBeenLastCalledWith("Checked 3 shows: 1 new episode", { severity: "success", linkToActivity: true });

    act(() => result.current.mutate({ tvdbId: "900" }));
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(2));
    expect(fetchMock.mock.calls[1][0]).toBe("/api/follows/900/check");
    expect(toast).toHaveBeenLastCalledWith("Some Show: searching for 1 now", { severity: "success", linkToActivity: true });
  });
});
