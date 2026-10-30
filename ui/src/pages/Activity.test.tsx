import type { ReactNode } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../contexts/ToastContext";
import type { ActivityItem } from "../hooks/useActivity";
import type { UpcomingItem } from "../hooks/useUpcoming";
import Activity from "./Activity";

vi.mock("../lib/logger", () => ({ logError: vi.fn() }));

const jsonResponse = (body: unknown, init?: ResponseInit) =>
  new Response(JSON.stringify(body), { status: 200, ...init });

const recent = new Date().toISOString();

const item = (over: Partial<ActivityItem> = {}): ActivityItem => ({
  id: 1,
  tvdb_id: "1001",
  media_title: "The Bear",
  category: "series",
  season: 3,
  episode: 1,
  is_anime: false,
  status: "downloading",
  created_at: recent,
  updated_at: recent,
  can_cancel: true,
  ...over,
});

const upcoming = (over: Partial<UpcomingItem> = {}): UpcomingItem => ({
  id: 50,
  tvdb_id: "3001",
  media_title: "The Bear",
  category: "series",
  season: 3,
  episode: 5,
  is_anime: false,
  release_time: "2099-01-01T12:00:00Z",
  due_at: "2099-01-01T12:00:00Z",
  attempts: 0,
  ...over,
});

const snapshot = {
  active: [
    item({ id: 1, tvdb_id: "1001", season: 3, episode: 1, status: "downloading" }),
    item({ id: 2, tvdb_id: "1002", season: 3, episode: 2, status: "downloading" }),
    item({ id: 3, tvdb_id: "1003", season: 3, episode: 3, status: "searching", reason: "No torrent found yet", next_attempt_at: "2099-01-01T12:00:00Z", attempts: 1, can_retry: true, can_pick_release: true }),
    item({ id: 4, tvdb_id: "2001", media_title: "Dune: Part Two", category: "movie", season: undefined, episode: undefined, status: "downloading", trace_id: "abcdef1234567890" }),
    // Has a schedule row below, so it must show up once, under Upcoming
    item({ id: 5, tvdb_id: "3001", season: 3, episode: 5, status: "scheduled", can_cancel: true }),
  ],
  recent: [
    item({ id: 6, tvdb_id: "4001", media_title: "Severance", season: 2, episode: 1, status: "completed", can_cancel: false }),
    item({ id: 7, tvdb_id: "4002", media_title: "Severance", season: 2, episode: 2, status: "failed", reason: "Not enough disk space", can_cancel: false, can_retry: true }),
    item({ id: 8, tvdb_id: "5001", media_title: "Andor", season: 2, episode: 4, status: "completed", can_cancel: false }),
  ],
  counts: {},
};

const upcomingRows = [upcoming({ id: 50, tvdb_id: "3001", episode: 5 }), upcoming({ id: 51, tvdb_id: "3002", episode: 6 })];

const health = {
  indexers: [
    { id: 1, name: "1337x", enabled: true, healthy: true, ip_blocked: false },
    { id: 2, name: "NYAA", enabled: true, healthy: false, failure: "Timed out", ip_blocked: true },
  ],
  relay: { configured: true, connected: true, relay: "se-sto-wg-001", ip: "203.0.113.9", location: "Stockholm", last_rotation_reason: "1337x blocked" },
  events: [{ at: recent, kind: "rotation", message: "Rotated to se-sto-wg-001", ok: true }],
  auto_rotate: true,
  queue: [
    { id: 1, limits: { per_minute: 6, burst: 2 }, tokens: 1, in_flight: "Severance S01E04", queued: 3, queued_by_priority: { manual: 1, follow: 2 }, searches_last_minute: 2 },
    { id: 2, limits: { per_minute: 6, burst: 2 }, tokens: 2, queued: 0, searches_last_minute: 0 },
  ],
};

function stubApi(over: { upcoming?: UpcomingItem[] } = {}) {
  const fetchMock = vi.fn((input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    if (url.startsWith("/api/activity")) return Promise.resolve(jsonResponse(snapshot));
    if (url.startsWith("/api/schedule/upcoming")) return Promise.resolve(jsonResponse(over.upcoming ?? upcomingRows));
    if (url.startsWith("/api/indexers/health")) return Promise.resolve(jsonResponse(health));
    return Promise.resolve(jsonResponse({ error: `unexpected ${url}` }, { status: 404 }));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <ToastContext.Provider value={vi.fn()}>{children}</ToastContext.Provider>
    </QueryClientProvider>
  );
  return render(<Activity />, { wrapper });
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("Activity page", () => {
  it("folds a show's episodes into one line and opens it on click", async () => {
    const fetchMock = stubApi();
    renderPage();

    // Three Bear episodes are in flight; the scheduled one belongs to Upcoming
    const bear = await screen.findByRole("button", { name: /^The Bear/ });
    // Grouping makes a long list cheap, so ask for far more than the server's default 200
    expect(fetchMock).toHaveBeenCalledWith("/api/activity?limit=1000");
    expect(bear).toHaveAttribute("aria-expanded", "false");
    expect(within(bear).getByText("2 downloading")).toBeInTheDocument();
    expect(within(bear).getByText("1 searching")).toBeInTheDocument();
    expect(screen.queryByText("S03E01")).not.toBeInTheDocument();

    // A movie is a single line with its own actions
    expect(screen.getByText("Dune: Part Two")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel Dune: Part Two Movie" })).toBeInTheDocument();

    await userEvent.click(bear);
    expect(bear).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("S03E01")).toBeInTheDocument();
    // The group line quotes the latest row too, so look at the row's own status text
    expect(screen.getByTitle(/^Retrying .* \(attempt 2\) · No torrent found yet$/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry The Bear Season 03 Episode 03 now" })).toBeInTheDocument();
  });

  it("counts every item once across the tabs and tiles", async () => {
    stubApi();
    renderPage();

    expect(await screen.findByRole("tab", { name: "In flight (4)" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Upcoming (2)" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Finished (3)" })).toBeInTheDocument();

    const tiles = screen.getAllByRole("button", { pressed: false }).filter((el) => el.tagName === "BUTTON" && el.hasAttribute("aria-pressed"));
    expect(tiles.map((tile) => tile.textContent)).toEqual([
      "3DownloadingDownloading",
      "1SearchingLooking for a torrent",
      "2ScheduledWaiting for release",
      "1FailedGave up",
    ]);
  });

  it("jumps to the Finished tab filtered to failed items from the Failed tile", async () => {
    stubApi();
    renderPage();

    await screen.findByRole("tab", { name: "Finished (3)" });
    await userEvent.click(screen.getByRole("button", { name: /Failed/ }));

    expect(screen.getByRole("tab", { name: "Finished (3)" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("Failed only")).toBeInTheDocument();
    expect(screen.getByText("Severance")).toBeInTheDocument();
    expect(screen.getByText("Not enough disk space")).toBeInTheDocument();
    expect(screen.queryByText("Andor")).not.toBeInTheDocument();

    // Clearing the filter brings the rest of the tab back, grouped
    const chip = screen.getByText("Failed only").closest(".MuiChip-root") as HTMLElement;
    await userEvent.click(within(chip).getByTestId("CancelIcon"));
    expect(await screen.findByText("Andor")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Severance/ })).toHaveAttribute("aria-expanded", "false");
  });

  it("lists upcoming rows by show with when the next one releases", async () => {
    stubApi();
    renderPage();

    await userEvent.click(await screen.findByRole("tab", { name: "Upcoming (2)" }));
    const bear = screen.getByRole("button", { name: /^The Bear/ });
    expect(bear).toHaveTextContent("2 episodes");
    expect(bear).toHaveTextContent(/S03E05 Releases on/);
    expect(screen.getByRole("button", { name: "Expand all" })).toBeInTheDocument();
  });

  it("filters a tab from its tile, and clears the filter on a second click or a tab change", async () => {
    stubApi();
    renderPage();

    const bear = await screen.findByRole("button", { name: /^The Bear/ });
    expect(within(bear).getByText("1 searching")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /^3\s*Downloading/ }));
    expect(screen.getByText("Downloading only")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^3\s*Downloading/ })).toHaveAttribute("aria-pressed", "true");
    expect(within(screen.getByRole("button", { name: /^The Bear/ })).queryByText("1 searching")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /^3\s*Downloading/ }));
    expect(screen.queryByText("Downloading only")).not.toBeInTheDocument();
    expect(within(screen.getByRole("button", { name: /^The Bear/ })).getByText("1 searching")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /^3\s*Downloading/ }));
    await userEvent.click(screen.getByRole("tab", { name: "Finished (3)" }));
    expect(screen.queryByText("Downloading only")).not.toBeInTheDocument();

    // The scheduled tile is a plain shortcut to Upcoming
    await userEvent.click(screen.getByRole("button", { name: /^2\s*Scheduled/ }));
    expect(screen.getByRole("tab", { name: "Upcoming (2)" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("button", { name: /^2\s*Scheduled/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByText(/ only$/)).not.toBeInTheDocument();
  });

  it("opens and closes every group from the toolbar", async () => {
    stubApi();
    renderPage();

    await screen.findByRole("button", { name: /^The Bear/ });
    await userEvent.click(screen.getByRole("button", { name: "Expand all" }));
    expect(screen.getByText("S03E01")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^The Bear/ })).toHaveAttribute("aria-expanded", "true");

    await userEvent.click(screen.getByRole("button", { name: "Collapse all" }));
    await waitFor(() => expect(screen.queryByText("S03E01")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Expand all" })).toBeInTheDocument();
  });

  it("caps Upcoming at twelve shows until Show all", async () => {
    const many = Array.from({ length: 13 }, (_, index) =>
      upcoming({ id: 100 + index, tvdb_id: `9${index}`, media_title: `Show ${index}`, release_time: `2099-02-${String(index + 1).padStart(2, "0")}T12:00:00Z`, due_at: `2099-02-${String(index + 1).padStart(2, "0")}T12:00:00Z` }),
    );
    stubApi({ upcoming: many });
    renderPage();

    await userEvent.click(await screen.findByRole("tab", { name: "Upcoming (13)" }));
    expect(screen.getAllByText(/^Show \d+$/)).toHaveLength(12);
    expect(screen.queryByText("Show 12")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Show all 13" }));
    expect(screen.getByText("Show 12")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show fewer" })).toBeInTheDocument();
  });

  it("keeps the indexer strip to one line until expanded", async () => {
    stubApi();
    renderPage();

    expect(await screen.findByText("NYAA · disabled")).toBeInTheDocument();
    expect(screen.getByText("via se-sto-wg-001 · 203.0.113.9")).toBeInTheDocument();
    expect(screen.getByText("1 searching · 3 waiting")).toBeInTheDocument();
    expect(screen.queryByText(/Stockholm/)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Show indexer details" }));
    await waitFor(() => expect(screen.getByText(/Stockholm/)).toBeInTheDocument());
    expect(screen.getByText(/Rotated to se-sto-wg-001/)).toBeInTheDocument();
    // Only the busy indexer gets a queue line
    expect(screen.getByText("1337x: searching Severance S01E04 · 3 waiting (manual 1, follow 2) · 2/6 per min")).toBeInTheDocument();
    expect(screen.queryByText(/^NYAA:/)).not.toBeInTheDocument();
  });
});
