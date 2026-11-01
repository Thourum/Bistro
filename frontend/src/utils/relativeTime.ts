const MILLISECONDS_PER_SECOND = 1000;
const SECONDS_PER_MINUTE = 60;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;

/** Formats elapsed time using the largest whole unit. */
export function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const sec = Math.floor(diff / MILLISECONDS_PER_SECOND);
  if (sec < SECONDS_PER_MINUTE) return `${sec}s ago`;
  const min = Math.floor(sec / SECONDS_PER_MINUTE);
  if (min < MINUTES_PER_HOUR) return `${min}m ago`;
  const hour = Math.floor(min / MINUTES_PER_HOUR);
  if (hour < HOURS_PER_DAY) return `${hour}h ago`;
  const day = Math.floor(hour / HOURS_PER_DAY);
  return `${day}d ago`;
}
