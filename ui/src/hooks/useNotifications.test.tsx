import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useClearFinished } from "./useNotifications";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useClearFinished", () => {
  it("deletes finished notifications and refreshes both the bell and the Activity page", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ count: 2 }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useClearFinished(), { wrapper });
    act(() => result.current.mutate());

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetchMock).toHaveBeenCalledWith("/api/notifications", expect.objectContaining({ method: "DELETE" }));
    const keys = invalidate.mock.calls.map(([opts]) => (opts as { queryKey: unknown[] }).queryKey);
    expect(keys).toEqual(expect.arrayContaining([["notifications"], ["activity"]]));
  });
});
