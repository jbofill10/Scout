import { render, screen } from "@testing-library/react";
import MediaCard from "./MediaCard";
import { pickCardRating } from "../../utils/ratings";

const media = { id: "1", name: "Sinners", imageUrl: "https://artworks.thetvdb.com/p.jpg" };

describe("MediaCard", () => {
  it("shows the title's headline score as a pill", () => {
    const rating = pickCardRating([{ source: "rotten_tomatoes", value: 97 }, { source: "imdb", value: 7.7 }], "movie");
    render(<MediaCard media={media} onClick={() => {}} rating={rating} />);

    const pill = screen.getByRole("img", { name: "Tomatometer 97%" });
    expect(pill).toHaveTextContent("97%");
  });

  it("shows IMDb for a series", () => {
    const rating = pickCardRating([{ source: "imdb", value: 8.7 }], "series");
    render(<MediaCard media={media} onClick={() => {}} rating={rating} />);

    expect(screen.getByRole("img", { name: "IMDb 8.7" })).toHaveTextContent("8.7");
  });

  it("renders no pill without a rating", () => {
    render(<MediaCard media={media} onClick={() => {}} rating={null} />);

    expect(screen.queryByRole("img", { name: /Tomatometer|IMDb|Metascore/ })).toBeNull();
    expect(screen.getByRole("button", { name: "View details for Sinners" })).toBeInTheDocument();
  });

  it("keeps the status badge next to the rating", () => {
    const rating = pickCardRating([{ source: "imdb", value: 8.7 }], "series");
    render(
      <MediaCard
        media={media}
        onClick={() => {}}
        rating={rating}
        statusBadge={{ type: "show", episodeCount: { downloaded: 3, aired: 10, upcoming: 0 } }}
      />,
    );

    expect(screen.getByText("3/10")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "IMDb 8.7" })).toBeInTheDocument();
  });

  it("counts a show against its aired episodes and tells the rest in the tooltip", () => {
    render(
      <MediaCard
        media={media}
        onClick={() => {}}
        statusBadge={{ type: "show", episodeCount: { downloaded: 11, aired: 11, upcoming: 9 } }}
      />,
    );

    const pill = screen.getByText("11/11");
    expect(pill).toHaveAttribute("title", "11 of 11 aired episodes in your library · 9 upcoming");
    expect(screen.queryByText("11/20")).toBeNull();
  });

  it("labels a movie in the library", () => {
    render(<MediaCard media={media} onClick={() => {}} statusBadge={{ type: "movie", inLibrary: true }} />);

    expect(screen.getByText("In Library")).not.toHaveAttribute("title");
  });
});
