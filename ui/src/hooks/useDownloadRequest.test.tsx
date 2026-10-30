import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../contexts/ToastContext";
import { describeSummary, useDownloadRequest, type DownloadSummary } from "./useDownloadRequest";

vi.mock("../lib/logger", () => ({ logError: vi.fn() }));
import { logError } from "../lib/logger";

const summary = (over: Partial<DownloadSummary> = {}): DownloadSummary => ({
  media_title: "Severance",
  category: "series",
  queued_now: 2,
  scheduled: 0,
  skipped: 0,
  ...over,
});

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
  const hook = renderHook(() => useDownloadRequest(), { wrapper });
  return { ...hook, toast, invalidate };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("useDownloadRequest", () => {
  it("posts a show to /api/shows and toasts the returned summary", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(summary()));
    vi.stubGlobal("fetch", fetchMock);
    const { result, toast, invalidate } = setup();
    const media = { id: "1", mediaName: "Severance" };

    act(() => result.current.requestDownload({ media, mediaType: "series", title: "Severance" }));

    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/shows");
    expect(init.method).toBe("POST");
    expect(init.headers).toEqual({ "Content-Type": "application/json" });
    expect(JSON.parse(init.body)).toEqual(media);

    expect(toast).toHaveBeenCalledWith("Severance: searching for 2 episodes now", {
      severity: "success",
      linkToActivity: true,
    });
    const keys = invalidate.mock.calls.map(([arg]) => arg?.queryKey);
    expect(keys).toEqual([["activity"], ["notifications"], ["schedule", "weekly"]]);
  });

  it("posts a movie to /api/movies", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse(summary({ category: "movie", media_title: "Alien", queued_now: 1 })));
    vi.stubGlobal("fetch", fetchMock);
    const { result, toast } = setup();

    act(() => result.current.requestDownload({ media: { id: "9" }, mediaType: "movie" }));

    await waitFor(() => expect(toast).toHaveBeenCalled());
    expect(fetchMock.mock.calls[0][0]).toBe("/api/movies");
    expect(toast).toHaveBeenCalledWith("Alien: searching for a torrent now", {
      severity: "success",
      linkToActivity: true,
    });
  });

  it("uses an info toast without an activity link when nothing new was accepted", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonResponse(summary({ queued_now: 0, skipped: 3 }))),
    );
    const { result, toast } = setup();

    act(() => result.current.requestDownload({ media: {}, mediaType: "series" }));

    await waitFor(() => expect(toast).toHaveBeenCalled());
    expect(toast).toHaveBeenCalledWith("Severance: already tracked — nothing new to download", {
      severity: "info",
      linkToActivity: false,
    });
  });

  it("reports a non-2xx response as an error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("boom", { status: 500, statusText: "Server Error" })),
    );
    const { result, toast, invalidate } = setup();

    act(() =>
      result.current.requestDownload({
        media: {},
        mediaType: "series",
        title: "Severance",
        component: "MediaCard",
      }),
    );

    await waitFor(() => expect(toast).toHaveBeenCalled());
    expect(toast).toHaveBeenCalledWith(
      'Couldn\'t start the download for "Severance". Please try again.',
      { severity: "error" },
    );
    expect(logError).toHaveBeenCalledWith(
      "Download request failed",
      expect.objectContaining({ message: "Download request failed: 500 Server Error" }),
      { component: "MediaCard", mediaType: "series", title: "Severance" },
    );
    expect(invalidate).not.toHaveBeenCalled();
    await waitFor(() => expect(result.current.isPending).toBe(false));
  });

  it("reports a network failure as an error, without a title in the message", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("offline")));
    const { result, toast } = setup();

    act(() => result.current.requestDownload({ media: {}, mediaType: "movie" }));

    await waitFor(() => expect(toast).toHaveBeenCalled());
    expect(toast).toHaveBeenCalledWith("Couldn't start the download. Please try again.", {
      severity: "error",
    });
    expect(logError).toHaveBeenCalledWith("Download request failed", expect.any(TypeError), {
      component: "unknown",
      mediaType: "movie",
      title: "",
    });
  });

  it("exposes isPending while a request is in flight", async () => {
    let resolve!: (r: Response) => void;
    const fetchMock = vi.fn().mockReturnValue(new Promise<Response>((r) => (resolve = r)));
    vi.stubGlobal("fetch", fetchMock);
    const { result, toast } = setup();
    expect(result.current.isPending).toBe(false);

    act(() => result.current.requestDownload({ media: {}, mediaType: "series" }));
    await waitFor(() => expect(result.current.isPending).toBe(true));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(toast).not.toHaveBeenCalled();

    await act(async () => resolve(jsonResponse(summary())));
    await waitFor(() => expect(result.current.isPending).toBe(false));
    expect(toast).toHaveBeenCalledTimes(1);
  });
});

describe("describeSummary", () => {
  it.each<[string, Partial<DownloadSummary>, string | undefined, string]>([
    ["single episode", { queued_now: 1 }, undefined, "Severance: searching for 1 episode now"],
    ["scheduled without date", { queued_now: 0, scheduled: 3 }, undefined, "Severance: 3 episodes scheduled for release"],
    ["queued and scheduled", { queued_now: 1, scheduled: 1 }, undefined, "Severance: searching for 1 episode now, 1 episode scheduled for release"],
    ["scheduled movie", { category: "movie", queued_now: 0, scheduled: 1 }, undefined, "Severance: 1 movie scheduled for release"],
    ["skipped only", { queued_now: 0, skipped: 2 }, undefined, "Severance: already tracked — nothing new to download"],
    ["nothing at all", { queued_now: 0 }, undefined, "Severance: nothing to download"],
    ["fallback title", { media_title: "", queued_now: 0 }, "Fallback", "Fallback: nothing to download"],
    ["no title anywhere", { media_title: "", queued_now: 0 }, undefined, "Media: nothing to download"],
  ])("%s", (_n, over, fallback, expected) => {
    expect(describeSummary(summary(over), fallback)).toBe(expected);
  });

  it("includes the first release date when scheduled", () => {
    const next = "2026-12-25T00:00:00Z";
    expect(describeSummary(summary({ queued_now: 0, scheduled: 2, next_release: next }))).toBe(
      `Severance: 2 episodes scheduled for release — first on ${new Date(next).toLocaleDateString()}`,
    );
  });
});
