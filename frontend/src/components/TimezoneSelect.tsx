import { useMemo, useState } from "react";

function timezones(): string[] {
  const intl = Intl as typeof Intl & { supportedValuesOf?: (key: string) => string[] };
  if (typeof intl.supportedValuesOf === "function") {
    return intl.supportedValuesOf("timeZone");
  }
  return [
    "UTC",
    "America/New_York",
    "America/Chicago",
    "America/Denver",
    "America/Los_Angeles",
    "Europe/London",
    "Europe/Paris",
    "Asia/Kolkata",
    "Asia/Calcutta",
    "Asia/Tokyo",
    "Australia/Sydney",
  ];
}

export default function TimezoneSelect({
  value,
  onChange,
}: {
  value: string;
  onChange: (next: string) => void;
}) {
  const zones = useMemo(() => {
    const list = timezones();
    if (value && !list.includes(value)) return [value, ...list];
    return list;
  }, [value]);
  const [query, setQuery] = useState("");
  const filtered = zones.filter((z) => z.toLowerCase().includes(query.toLowerCase()));
  const currentIndex = Math.max(0, zones.indexOf(value));

  function step(delta: number) {
    const next = zones[(currentIndex + delta + zones.length) % zones.length];
    onChange(next);
    setQuery("");
  }

  return (
    <div className="tz-select">
      <div className="tz-current">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "ArrowUp") {
              e.preventDefault();
              step(-1);
            }
            if (e.key === "ArrowDown") {
              e.preventDefault();
              step(1);
            }
            if (e.key === "Enter" && filtered[0]) {
              e.preventDefault();
              onChange(filtered[0]);
              setQuery("");
            }
          }}
          placeholder={value || "Search timezone"}
          aria-label="Search timezone"
        />
        <div className="tz-steppers">
          <button type="button" aria-label="Previous timezone" onClick={() => step(-1)}>▲</button>
          <button type="button" aria-label="Next timezone" onClick={() => step(1)}>▼</button>
        </div>
      </div>
      <div className="muted tz-selected">Current: {value || "—"}</div>
      <select
        size={8}
        value={value}
        aria-label="Timezone list"
        onChange={(e) => {
          onChange(e.target.value);
          setQuery("");
        }}
      >
        {(query ? filtered : zones).map((z) => (
          <option key={z} value={z}>{z}</option>
        ))}
      </select>
    </div>
  );
}
