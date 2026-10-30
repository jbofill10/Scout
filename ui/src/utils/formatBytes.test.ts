import { formatBytes } from "./formatBytes";

describe("formatBytes", () => {
  it.each([
    [0, ""],
    [-5, ""],
    [512, "512 B"],
    [1024, "1.0 KB"],
    [1536, "1.5 KB"],
    [10 * 1024, "10 KB"],
    [1.4 * 1024 ** 3, "1.4 GB"],
    [25 * 1024 ** 3, "25 GB"],
    [3 * 1024 ** 4, "3.0 TB"],
  ])("%d -> %s", (bytes, expected) => {
    expect(formatBytes(bytes)).toBe(expected);
  });
});
