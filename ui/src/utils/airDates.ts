/**
 * TVDB air dates are plain calendar days (YYYY-MM-DD) with no timezone.
 * Parse them as local dates so a date never slips a day either way.
 */
export const parseAirDate = (value?: string): Date | null => {
  if (!value) return null;
  const [year, month, day] = value.split("-").map(Number);
  if (!year || !month || !day) return null;
  return new Date(year, month - 1, day);
};

/** "Jul 11, 2025" in the viewer's locale, or null when there is no usable date. */
export const formatAirDate = (value?: string): string | null => {
  const date = parseAirDate(value);
  if (!date) return null;
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
};

/**
 * Whether an air date is today or earlier.
 *
 * A blank or unparseable date has not aired: TVDB leaves the date empty on
 * announced episodes, and the backend will not search for those either. An
 * undefined date means the caller has no air information at all (status
 * straight from the library database), which reads as aired so coverage
 * still adds up.
 */
export const hasAired = (aired: string | undefined, today: Date = new Date()): boolean => {
  if (aired === undefined) return true;
  const date = parseAirDate(aired);
  if (!date) return false;
  const endOfToday = new Date(today.getFullYear(), today.getMonth(), today.getDate(), 23, 59, 59, 999);
  return date <= endOfToday;
};
