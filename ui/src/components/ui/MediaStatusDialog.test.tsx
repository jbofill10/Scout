import type { ReactNode } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastContext } from "../../contexts/ToastContext";
import type { EnrichedMedia } from "../../types/MediaStatus";
import { MediaStatusDialog } from "./MediaStatusDialog";

vi.mock("../../lib/logger", () => ({ logError: vi.fn() }));

const jsonResponse = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

const episode = (id: number, seasonNumber: number, number: number, aired: string) => ({
  id,
  seasonNumber,
  number,
  aired,
  name: `Episode ${number}`,
  image: "",
});

// Dexter: Resurrection mid-run as TVDB lists it: season 1 aired and in the
// library, season 2 announced with one dated episode and eight without.
const episodes = [
  episode(1, 0, 1, "2025-07-01"), // a special the library lacks; never part of the headline count
  ...Array.from({ length: 11 }, (_, i) => episode(100 + i, 1, i + 1, "2025-07-11")),
  episode(201, 2, 1, "2099-01-10"),
  ...Array.from({ length: 8 }, (_, i) => episode(202 + i, 2, i + 2, "")),
];

const enrichedMedia: EnrichedMedia = {
  media: {
    id: "900",
    mediaName: "Dexter: Resurrection",
    image_url: "p.jpg",
    metadata: { episodes },
  },
  status: {
    type: "series",
    downloaded: 11,
    aired: 11,
    total: 20,
    seasons: [
      {
        seasonNum: 1,
        episodes: Array.from({ length: 11 }, (_, i) => ({ episodeNum: i + 1, downloaded: true, tvdbId: String(100 + i) })),
      },
    ],
  },
};

const metadataStatus = { hasTvdbData: true, tvdbEpisodeCount: 11, plexEpisodeCount: 11, missingCount: 0, upcomingCount: 9 };

function renderDialog(media: EnrichedMedia = enrichedMedia, withDownload = false) {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (url.includes("/metadata-status")) return Promise.resolve(jsonResponse(metadataStatus));
      if (url.includes("/api/follows")) return Promise.resolve(jsonResponse([]));
      return Promise.resolve(jsonResponse({ id: 900, name: "Dexter: Resurrection" }));
    }),
  );
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <ToastContext.Provider value={vi.fn()}>{children}</ToastContext.Provider>
    </QueryClientProvider>
  );
  render(
    <MediaStatusDialog
      open
      onClose={() => {}}
      status={null}
      mediaName="Dexter: Resurrection"
      posterUrl="p.jpg"
      enrichedMedia={media}
      showDownloadButton={withDownload}
      onDownload={withDownload ? () => {} : undefined}
    />,
    { wrapper },
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("MediaStatusDialog episode coverage", () => {
  it("measures the library against aired episodes and lists the rest as upcoming", async () => {
    renderDialog();

    expect(await screen.findByText("11 of 11 aired episodes in your library · 9 upcoming")).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { name: "Library coverage" })).toHaveAttribute("aria-valuenow", "100");
    expect(screen.getByRole("tab", { name: "Specials · 0/1" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Season 1 · 11/11" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Season 2 · 9 upcoming" })).toBeInTheDocument();
    expect(screen.queryByText(/11 of 20/)).toBeNull();
  });

  it("says how many episodes are yet to air once everything aired is in", async () => {
    renderDialog();

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Caught up");
    expect(alert).toHaveTextContent("You have all 11 aired episodes in your library! 9 more are yet to air.");
  });

  it("marks unaired episodes as upcoming or TBA rather than missing", async () => {
    renderDialog();

    await userEvent.click(await screen.findByRole("tab", { name: "Season 2 · 9 upcoming" }));

    const panel = screen.getByRole("tabpanel");
    expect(within(panel).getAllByText("Upcoming")).toHaveLength(1);
    expect(within(panel).getAllByText("TBA")).toHaveLength(8);
    expect(within(panel).queryByText("Missing")).toBeNull();
  });

  it("offers to schedule upcoming episodes once everything aired is in", async () => {
    renderDialog(enrichedMedia, true);

    expect(await screen.findByRole("button", { name: /Schedule upcoming/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Download missing/ })).toBeNull();
  });

  it("still calls an aired episode the library lacks missing", async () => {
    const seasons = enrichedMedia.status.seasons!;
    const withGap: EnrichedMedia = {
      ...enrichedMedia,
      status: {
        ...enrichedMedia.status,
        downloaded: 10,
        seasons: [{ seasonNum: 1, episodes: seasons[0].episodes.filter((ep) => ep.episodeNum !== 5) }],
      },
    };
    renderDialog(withGap, true);

    expect(await screen.findByText("10 of 11 aired episodes in your library · 9 upcoming")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Download missing/ })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Season 1 · 10/11" })).toBeInTheDocument();
    const panel = screen.getByRole("tabpanel");
    expect(within(panel).getAllByText("In library")).toHaveLength(10);
    expect(within(panel).getAllByText("Missing")).toHaveLength(1);
  });
});
