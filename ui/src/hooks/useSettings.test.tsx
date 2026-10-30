import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../contexts/ToastContext";
import { SETTINGS_QUERY_KEY, useAddUploader, useSettings, useUpdateSettings, type SettingsView } from "./useSettings";

vi.mock("../lib/logger", () => ({ logError: vi.fn() }));

const view = (over: Partial<SettingsView["values"]> = {}): SettingsView => ({
  values: { min_quality: "1080p", subtitle_languages: ["en"], subtitle_prefer_sdh: false, plex_sync_delay: "2m0s", plex_sync_interval: "30m0s", ...over },
  defaults: { min_quality: "1080p", subtitle_languages: ["en"], subtitle_prefer_sdh: false, plex_sync_delay: "2m0s", plex_sync_interval: "30m0s" },
  overridden: {},
  options: { min_quality: ["480p", "720p", "1080p", "2160p"], subtitles_enabled: true },
});

const jsonResponse = (body: unknown, init?: ResponseInit) => new Response(JSON.stringify(body), { status: 200, ...init });

function setup() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const toast = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <ToastContext.Provider value={toast}>{children}</ToastContext.Provider>
    </QueryClientProvider>
  );
  return { wrapper, toast, queryClient };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("useSettings", () => {
  it("loads the settings view", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(view())));
    const { wrapper } = setup();
    const { result } = renderHook(() => useSettings(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.values.min_quality).toBe("1080p");
    expect(fetch).toHaveBeenCalledWith("/api/settings", undefined);
  });
});

describe("useUpdateSettings", () => {
  it("PUTs the subset, replaces the cached view and toasts", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(view({ min_quality: "720p" })));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast, queryClient } = setup();
    const { result } = renderHook(() => useUpdateSettings(), { wrapper });

    act(() => result.current.mutate({ min_quality: "720p" }));

    await waitFor(() => expect(toast).toHaveBeenCalledWith("Settings saved", { severity: "success" }));
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/settings");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body)).toEqual({ min_quality: "720p" });
    expect((queryClient.getQueryData(SETTINGS_QUERY_KEY) as SettingsView).values.min_quality).toBe("720p");
  });

  it("surfaces the server's validation message", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ error: "must be one of 480p, 720p, 1080p, 2160p", field: "min_quality" }, { status: 400 })));
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useUpdateSettings(), { wrapper });

    act(() => result.current.mutate({ min_quality: "900p" }));

    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    expect(toast).toHaveBeenCalledWith("must be one of 480p, 720p, 1080p, 2160p", { severity: "error" });
  });
});

describe("useAddUploader", () => {
  it("posts the preference and refreshes the list", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ id: 3, media_type: "movie", is_anime: false, uploader: "QxR" }, { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast, queryClient } = setup();
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useAddUploader(), { wrapper });

    act(() => result.current.mutate({ media_type: "movie", is_anime: false, uploader: "QxR" }));

    await waitFor(() => expect(toast).toHaveBeenCalledWith("Preferring QxR", { severity: "success" }));
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ media_type: "movie", is_anime: false, uploader: "QxR" });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["settings", "uploaders"] });
  });
});
