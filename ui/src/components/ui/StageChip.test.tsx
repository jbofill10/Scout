import { render, screen } from "@testing-library/react";
import { StageChip } from "./StageChip";

describe("StageChip", () => {
  it.each([
    ["scheduled", "Scheduled"],
    ["searching", "Searching"],
    ["downloading", "Downloading"],
    ["completed", "Completed"],
    ["failed", "Failed"],
    [undefined, "Unknown"],
    ["bogus", "Bogus"],
  ])("renders the label for stage %s", (stage, label) => {
    render(<StageChip stage={stage} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("lets a custom label override the stage name", () => {
    render(<StageChip stage="searching" label="Retrying" />);
    expect(screen.getByText("Retrying")).toBeInTheDocument();
    expect(screen.queryByText("Searching")).not.toBeInTheDocument();
  });
});
