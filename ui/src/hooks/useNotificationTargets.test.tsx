import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../contexts/ToastContext";
import { useCreateTarget, useTestTarget, useUpdateTarget, type NotificationTarget } from "./useNotificationTargets";

vi.mock("../lib/logger", () => ({ logError: vi.fn() }));

const jsonResponse = (body: unknown, init?: ResponseInit) => new Response(JSON.stringify(body), { status: 200, ...init });

const target: NotificationTarget = {
  id: 3, name: "Phone", kind: "ntfy", url: "https://ntfy.sh/scout", has_token: false, events: ["failed"], enabled: true, created_at: "",
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

describe("notification target hooks", () => {
  it("creates a target", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(target, { status: 201 }));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useCreateTarget(), { wrapper });

    act(() => result.current.mutate({ name: "Phone", kind: "ntfy", url: "https://ntfy.sh/scout", events: ["failed"] }));

    await waitFor(() => expect(toast).toHaveBeenCalledWith("Added Phone. Send a test to check it.", { severity: "success" }));
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/notification-targets");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({ name: "Phone", kind: "ntfy", url: "https://ntfy.sh/scout", events: ["failed"] });
  });

  it("updates without sending a token when none was typed", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(target));
    vi.stubGlobal("fetch", fetchMock);
    const { wrapper } = setup();
    const { result } = renderHook(() => useUpdateTarget(), { wrapper });

    act(() => result.current.mutate({ id: 3, name: "Phone", kind: "ntfy", url: "https://ntfy.sh/scout", events: ["failed"], enabled: false }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/notification-targets/3");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body)).not.toHaveProperty("token");
    expect(JSON.parse(init.body).enabled).toBe(false);
  });

  it("reports a failed test with the server's reason", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ ok: false, error: "ntfy answered 403: forbidden" }, { status: 502 })));
    const { wrapper, toast } = setup();
    const { result } = renderHook(() => useTestTarget(), { wrapper });
    act(() => result.current.mutate(target));
    await waitFor(() => expect(toast).toHaveBeenCalledWith("Phone: ntfy answered 403: forbidden", { severity: "error" }));
  });
});
