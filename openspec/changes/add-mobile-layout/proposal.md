# Change: Mobile Layout

## Why

The UI was laid out for a 1920x1080 desktop and nothing adapted below that.
On a phone held upright the five navigation pills overflowed the top bar, the
search overlay's media-type toggle left the search field a few characters wide,
poster grids collapsed to one enormous column (or, on the Library page, one
180px column beside empty space), and the episode dialog was a 326px-wide card
whose list showed three rows. The only workable way to use Scout on a phone was
to rotate it sideways.

## What Changes

- **Bottom tab bar on phones and tablets.** Below the `md` breakpoint the
  navbar's centre pills hide and a fixed bottom navigation with the same five
  routes (and the same in-flight badge on Activity) takes over. Pages reserve
  room for it.
- **Poster rows and grids size themselves to the screen.** Carousels show ~2.6
  posters on a phone, bleed to the screen edge, and scroll by touch; the
  pointer-only nav buttons no longer sit invisibly over the first and last
  poster on touch screens. The search results grid and the Library grid always
  fit two columns on a phone.
- **Search overlay fills a phone screen**, with the media-type toggle on its own
  row under the search field.
- **Episode dialog goes full screen** on phone-width or short viewports, with
  the season tabs pinned and only the episode list scrolling. Smaller dialogs
  keep a 16px gutter instead of MUI's 32px.
- **Page furniture adapts**: page headers stack their action below the title,
  the Activity stage tiles go two-up, Activity rows drop the right-hand column
  and show the time inline, and the Library page picks up the shared page
  frame and header used by every other route.
- **Hover-only effects are limited to devices that can hover**, so a tapped
  poster does not stay raised.

## Impact

- Affected specs: new `ui-layout` capability
- Affected code: `ui/src/App.tsx`, `ui/src/theme/theme.ts`, `ui/src/index.css`,
  `ui/index.html`, `ui/src/components/ui/{BottomNav,navLinks,carouselItem,
  Navbar,HorizontalCarousel,GenreRow,MediaCard,ScheduleWidget,PageHeader,
  SearchDropdown,SearchResultsGrid,MediaStatusDialog,ToastProvider}`,
  `ui/src/pages/{Home,Shows,Movies,Search,Library,Activity}`
- No API or backend changes
