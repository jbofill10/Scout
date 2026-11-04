import { render, screen } from "@testing-library/react";
import RatingPill from "./RatingPill";
import { formatRating } from "../../utils/ratings";

const model = (source: string, value: number) => {
  const rating = formatRating({ source, value });
  if (!rating) throw new Error(`unknown source ${source}`);
  return rating;
};

describe("RatingPill", () => {
  it("names the Tomatometer and shows the percentage", () => {
    render(<RatingPill rating={model("rotten_tomatoes", 97)} />);
    expect(screen.getByRole("img", { name: "Tomatometer 97%" })).toHaveTextContent("97%");
  });

  it("draws a splat for a rotten score", () => {
    const { container } = render(<RatingPill rating={model("rotten_tomatoes", 41)} />);
    expect(screen.getByRole("img", { name: "Tomatometer 41%" })).toBeInTheDocument();
    expect(container.querySelector("circle")).toBeNull();
    expect(container.querySelector("svg path")).not.toBeNull();
  });

  it("draws a tomato for a fresh score", () => {
    const { container } = render(<RatingPill rating={model("rotten_tomatoes", 60)} />);
    expect(container.querySelector("circle")).not.toBeNull();
  });

  it("shows the Metascore with its colour swatch", () => {
    render(<RatingPill rating={model("metacritic", 84)} size="detail" />);
    expect(screen.getByRole("img", { name: "Metascore 84" })).toHaveTextContent("84");
  });

  it("shows IMDb with a star", () => {
    const { container } = render(<RatingPill rating={model("imdb", 8.7)} />);
    expect(screen.getByRole("img", { name: "IMDb 8.7" })).toHaveTextContent("8.7");
    expect(container.querySelector("[data-testid='StarRoundedIcon']")).not.toBeNull();
  });
});
