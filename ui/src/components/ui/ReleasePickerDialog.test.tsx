import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import ReleasePickerDialog from "./ReleasePickerDialog";
import type { ReleaseCandidate } from "../../hooks/useDownloadControls";

const jsonResponse = (body: unknown, init?: ResponseInit) =>
  new Response(JSON.stringify(body), { status: 200, ...init });

const candidate = (over: Partial<ReleaseCandidate> = {}): ReleaseCandidate => ({
  title: "Some.Show.S01E05.1080p.WEB.x264-GRP",
  indexer: "1337x",
  indexer_id: 2,
  guid: "magnet:?xt=urn:btih:abc",
  info_hash: "abc",
  size: 1_500_000_000,
  seeders: 40,
  leechers: 3,
  publish_date: new Date().toISOString(),
  quality: "1080p",
  accepted: true,
  score: 2.1,
  previously_failed: false,
  query: "Some Show S01E05",
  ...over,
});

function renderDialog(props: Partial<React.ComponentProps<typeof ReleasePickerDialog>> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  const onGrab = vi.fn();
  const onClose = vi.fn();
  render(
    <ReleasePickerDialog itemId="55" title="Some Show" detail="Season 01 Episode 05" open onClose={onClose} onGrab={onGrab} {...props} />,
    { wrapper },
  );
  return { onGrab, onClose };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ReleasePickerDialog", () => {
  it("lists candidates with their verdicts and grabs the chosen one", async () => {
    const accepted = candidate();
    const rejected = candidate({
      title: "Some.Show.S01E05.720p.WEB.x264-GRP",
      guid: "magnet:?xt=urn:btih:def",
      info_hash: "def",
      quality: "720p",
      accepted: false,
      reason: "720p is below the minimum quality",
      previously_failed: true,
      seeders: 300,
    });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ item_id: "55", media_title: "Some Show", queries: 4, candidates: [accepted, rejected] })));
    const { onGrab } = renderDialog();

    await waitFor(() => expect(screen.getAllByTestId("release-candidate")).toHaveLength(2));
    expect(screen.getByText(/2 releases from 4 searches, 1 Scout would accept/)).toBeInTheDocument();
    expect(screen.getByText(/Scout would take this \(score 2.10\)/)).toBeInTheDocument();
    expect(screen.getByText(/Passed over: 720p is below the minimum quality/)).toBeInTheDocument();
    expect(screen.getByText("Tried before")).toBeInTheDocument();
    expect(screen.getByText(/1.4 GB · 40 seeds · 1337x/)).toBeInTheDocument();

    const buttons = screen.getAllByRole("button", { name: /^Download$/ });
    await userEvent.click(buttons[1]);
    expect(onGrab).toHaveBeenCalledWith(rejected);
  });

  it("shows the empty state with the number of searches", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ item_id: "55", media_title: "Some Show", queries: 6, candidates: [] })));
    renderDialog();

    expect(await screen.findByText("No releases found")).toBeInTheDocument();
    expect(screen.getByText("6 searches came back empty.")).toBeInTheDocument();
  });

  it("surfaces the server's error and offers a retry", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ error: "Could not search for releases" }, { status: 502 }))
      .mockResolvedValueOnce(jsonResponse({ item_id: "55", media_title: "Some Show", queries: 1, candidates: [candidate()] }));
    vi.stubGlobal("fetch", fetchMock);
    renderDialog();

    expect(await screen.findByText("Couldn't search for releases")).toBeInTheDocument();
    expect(screen.getByText("Could not search for releases")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(screen.getAllByTestId("release-candidate")).toHaveLength(1));
  });

  it("does not search while closed", () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    renderDialog({ open: false });
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
