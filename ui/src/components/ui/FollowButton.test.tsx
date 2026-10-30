import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../../contexts/ToastContext";
import FollowButton from "./FollowButton";

vi.mock("../../lib/logger", () => ({ logError: vi.fn() }));

const jsonResponse = (body: unknown, init?: ResponseInit) => new Response(JSON.stringify(body), { status: 200, ...init });

function renderButton() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const toast = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <ToastContext.Provider value={toast}>{children}</ToastContext.Provider>
    </QueryClientProvider>
  );
  render(<FollowButton media={{ id: "900", mediaName: "Some Show", image_url: "p" }} />, { wrapper });
  return { toast };
}

afterEach(() => vi.unstubAllGlobals());

describe("FollowButton", () => {
  it("offers the two follow modes and posts the chosen one", async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        return Promise.resolve(jsonResponse({ show: { tvdb_id: "900", title: "Some Show" }, check: { tvdb_id: "900", title: "Some Show", listed: 1, new: 0, queued_now: 0, scheduled: 0, checked_at: "" } }, { status: 201 }));
      }
      return Promise.resolve(jsonResponse([]));
    });
    vi.stubGlobal("fetch", fetchMock);
    renderButton();

    const button = await screen.findByRole("button", { name: "Follow Some Show" });
    await userEvent.click(button);
    await userEvent.click(await screen.findByText("Also grab missing episodes"));

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(([, init]) => init?.method === "POST");
      expect(post).toBeDefined();
      expect(JSON.parse(String(post?.[1]?.body))).toMatchObject({ include_missing: true, media: { id: "900" } });
    });
  });

  it("shows Following when the show is already followed", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse([{ tvdb_id: "900", title: "Some Show", followed_at: "", include_missing: false, episodes_added: 0 }])));
    renderButton();
    expect(await screen.findByRole("button", { name: "Unfollow Some Show" })).toHaveTextContent("Following");
  });
});
