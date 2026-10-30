import {
  FILTER_LABELS,
  SEARCH_EXAMPLES,
  filterLabel,
  needsTitle,
  withoutFilter,
  type SearchFilter,
  type SearchQuery,
} from "./searchQuery";

const query = (over: Partial<SearchQuery> = {}): SearchQuery => ({
  raw: "",
  title: "",
  filters: [],
  ...over,
});

describe("filterLabel", () => {
  it.each(Object.entries(FILTER_LABELS))("labels %s as %s", (key, label) => {
    expect(filterLabel(key)).toBe(label);
  });

  it.each([
    ["year", "Year"],
    ["director", "Director"],
    ["actor", "Actor"],
    ["person", "Person"],
    ["studio", "Studio"],
    ["network", "Network"],
    ["country", "Country"],
    ["language", "Language"],
    ["imdb", "IMDb"],
  ])("has the expected text for %s", (key, label) => {
    expect(filterLabel(key)).toBe(label);
  });

  it("falls back to the raw key for unknown filters", () => {
    expect(filterLabel("genre")).toBe("genre");
    expect(filterLabel("")).toBe("");
  });
});

describe("needsTitle", () => {
  const f = (key: string): SearchFilter => ({ key, value: "v" });

  it.each([
    ["year", true],
    ["studio", true],
    ["network", true],
    ["country", true],
    ["language", true],
    ["actor", false],
    ["person", false],
    ["director", false],
    ["imdb", false],
  ])("with only a %s filter and no title -> %s", (key, expected) => {
    expect(needsTitle(query({ filters: [f(key)] }))).toBe(expected);
  });

  it("is false when there is a title", () => {
    expect(needsTitle(query({ title: "Alien", filters: [f("year")] }))).toBe(false);
  });

  it("is false when there are no filters at all", () => {
    expect(needsTitle(query())).toBe(false);
  });

  it("is false when any filter can stand alone", () => {
    expect(needsTitle(query({ filters: [f("year"), f("actor")] }))).toBe(false);
  });
});

describe("withoutFilter", () => {
  const year: SearchFilter = { key: "year", value: "2022" };
  const actor: SearchFilter = { key: "actor", value: "Mia Goth" };
  const studio: SearchFilter = { key: "studio", value: "A24" };

  it.each([
    ["removes the first filter", [year, actor, studio], year, 'X actor:"Mia Goth" studio:A24'],
    ["removes a middle filter", [year, actor, studio], actor, "X year:2022 studio:A24"],
    ["removes the last filter", [year, actor, studio], studio, 'X year:2022 actor:"Mia Goth"'],
    ["removes the only filter", [year], year, "X"],
  ])("%s", (_name, filters, removed, expected) => {
    expect(withoutFilter(query({ title: "X", filters }), removed)).toBe(expected);
  });

  it("quotes values containing whitespace only", () => {
    const text = withoutFilter(query({ title: "X", filters: [actor, studio] }), year);
    expect(text).toBe('X actor:"Mia Goth" studio:A24');
  });

  it("leaves everything in place when the filter is not present", () => {
    const absent: SearchFilter = { key: "year", value: "2022" }; // equal but not the same object
    expect(withoutFilter(query({ title: "X", filters: [year] }), absent)).toBe("X year:2022");
    expect(withoutFilter(query({ title: "X", filters: [actor] }), year)).toBe('X actor:"Mia Goth"');
  });

  it("omits the title when empty", () => {
    expect(withoutFilter(query({ filters: [actor, studio] }), studio)).toBe('actor:"Mia Goth"');
  });

  it("returns an empty string when nothing remains", () => {
    expect(withoutFilter(query({ filters: [year] }), year)).toBe("");
  });
});

describe("SEARCH_EXAMPLES", () => {
  it("has non-empty text and hints with a valid media type", () => {
    expect(SEARCH_EXAMPLES.length).toBeGreaterThan(0);
    for (const ex of SEARCH_EXAMPLES) {
      expect(ex.text).not.toBe("");
      expect(ex.hint).not.toBe("");
      expect(["movie", "series"]).toContain(ex.mediaType);
    }
  });

  it("has unique text", () => {
    const texts = SEARCH_EXAMPLES.map((e) => e.text);
    expect(new Set(texts).size).toBe(texts.length);
  });

  it("uses only filter keys that have labels", () => {
    for (const ex of SEARCH_EXAMPLES) {
      const m = ex.text.match(/\b(\w+):/);
      if (m) expect(FILTER_LABELS).toHaveProperty(m[1]);
    }
  });
});
