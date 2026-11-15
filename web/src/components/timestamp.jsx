import { formatRelativeTime, formatTimestamp } from "../format.js";

// Timestamp renders an RFC 3339 timestamp for people: relative ("3 hours
// ago") by default, or absolute in the viewer's locale with `absolute`. The
// other form is shown on hover.
export function Timestamp({ value, absolute = false, className }) {
  if (!value) return null;
  const abs = formatTimestamp(value);
  return (
    <time dateTime={value} title={absolute ? value : abs} className={className}>
      {absolute ? abs : formatRelativeTime(value)}
    </time>
  );
}
