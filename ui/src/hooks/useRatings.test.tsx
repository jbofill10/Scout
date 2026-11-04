import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { chunkRatingsItems, useRatingsBatch } from "./useRatings";
import type { RatingsItem } from "./useRatings";

const items: RatingsItem[] = Array.from({ length: 45 }, (_, i) => ({
  id: String(i + 1),
  mediaType: i % 2 === 0 ? "series" : "movie",
}));

afterEach(() => vi.unstubAllGlobals());

describe("chunkRatingsItems", () => {
  it("splits into chunks of the given size", () => {
    expect(chunkRatingsItems(items, 40).map((c) => c.length)).toEqual([40, 5]);
    expect(chunkRatingsItems([], 40)).toEqual([]);
  });
});

describe("useRatingsBatch", () => {
  it("posts each chunk and folds the answers into one map", async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) => {
      const requested = JSON.parse(String(init?.body)) as RatingsItem[];
      const rows = requested.map((item) =>
        item.id === "3"
          ? { id: item.id, mediaType: item.mediaType }
          : { id: item.id, mediaType: item.mediaType, imdbId: `tt${item.id}`, ratings: [{ source: "imdb", value: 7 }] },
      );
      return Promise.resolve(new Response(JSON.stringify(rows), { status: 200 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useRatingsBatch(items), { wrapper });

    await waitFor(() => expect(result.current.size).toBe(44));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/ratings/batch");
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toHaveLength(40);
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toHaveLength(5);
    expect(result.current.get("1")).toEqual([{ source: "imdb", value: 7 }]);
    expect(result.current.has("3")).toBe(false);
  });

  it("asks nothing when disabled", () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const queryClient = new QueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useRatingsBatch(items, false), { wrapper });

    expect(result.current.size).toBe(0);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
