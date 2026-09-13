/**
 * TVDB serves a quarter-size copy of every poster at the same path with a
 * `_t` suffix (roughly 45KB instead of 100-500KB). Cards render at ~200px
 * wide, so the thumbnail is all they need. Callers keep the full poster as a
 * fallback for the rare artwork that has no thumbnail.
 *
 * Non-TVDB URLs (Plex thumbs, data URIs) are returned unchanged.
 */
export function toThumbnailUrl(url: string): string {
  if (!/^https?:\/\/artworks\.thetvdb\.com\/banners\//i.test(url)) {
    return url;
  }
  if (/_t\.[a-z0-9]+$/i.test(url)) {
    return url;
  }
  return url.replace(/\.([a-z0-9]+)$/i, "_t.$1");
}
