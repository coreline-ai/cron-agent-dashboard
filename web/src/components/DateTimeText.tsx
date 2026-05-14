import { useEffect, useState } from 'react';

export type DateTimeTextProps = {
  value?: string;
  mode?: 'absolute' | 'relative' | 'both';
  empty?: string;
};

const absoluteFormatter = new Intl.DateTimeFormat('ko-KR', {
  dateStyle: 'medium',
  timeStyle: 'short'
});

const relativeFormatter = new Intl.RelativeTimeFormat('ko-KR', {
  numeric: 'auto'
});

export function DateTimeText({ value, mode = 'absolute', empty = '-' }: DateTimeTextProps) {
  const [now, setNow] = useState(() => Date.now());
  const trimmedValue = value?.trim();

  useEffect(() => {
    if (!trimmedValue || mode === 'absolute') {
      return;
    }

    const parsedDate = new Date(trimmedValue);
    if (Number.isNaN(parsedDate.getTime())) {
      return;
    }

    setNow(Date.now());
    const intervalId = window.setInterval(() => setNow(Date.now()), 60_000);
    return () => window.clearInterval(intervalId);
  }, [mode, trimmedValue]);

  if (!trimmedValue) {
    return <span className="date-time-text date-time-text--empty">{empty}</span>;
  }

  const date = new Date(trimmedValue);
  const time = date.getTime();

  if (Number.isNaN(time)) {
    return <span className="date-time-text date-time-text--invalid">{trimmedValue}</span>;
  }

  const absolute = absoluteFormatter.format(date);
  const relative = formatRelativeTime(time, now);
  const text = mode === 'relative' ? relative : mode === 'both' ? `${absolute} (${relative})` : absolute;

  return (
    <time className={`date-time-text date-time-text--${mode}`} dateTime={date.toISOString()} title={mode === 'absolute' ? relative : absolute}>
      {text}
    </time>
  );
}

function formatRelativeTime(time: number, baseTime: number) {
  const diffMs = time - baseTime;
  const absMs = Math.abs(diffMs);
  const minuteMs = 60_000;
  const hourMs = 60 * minuteMs;
  const dayMs = 24 * hourMs;
  const weekMs = 7 * dayMs;
  const monthMs = 30 * dayMs;
  const yearMs = 365 * dayMs;

  if (absMs < minuteMs) {
    return relativeFormatter.format(Math.round(diffMs / 1_000), 'second');
  }
  if (absMs < hourMs) {
    return relativeFormatter.format(Math.round(diffMs / minuteMs), 'minute');
  }
  if (absMs < dayMs) {
    return relativeFormatter.format(Math.round(diffMs / hourMs), 'hour');
  }
  if (absMs < weekMs) {
    return relativeFormatter.format(Math.round(diffMs / dayMs), 'day');
  }
  if (absMs < monthMs) {
    return relativeFormatter.format(Math.round(diffMs / weekMs), 'week');
  }
  if (absMs < yearMs) {
    return relativeFormatter.format(Math.round(diffMs / monthMs), 'month');
  }
  return relativeFormatter.format(Math.round(diffMs / yearMs), 'year');
}
