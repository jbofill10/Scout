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
        statusBadge={{ type: "show", episodeCount: { downloaded: 3, total: 10 } }}
      />,
    );

    expect(screen.getByText("3/10")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "IMDb 8.7" })).toBeInTheDocument();
  });
});
