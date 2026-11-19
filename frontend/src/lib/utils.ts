import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * Merge conditional class names and resolve Tailwind conflicts.
 * Used by all shadcn/ui-style components.
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}

/** Format a latency value (ms) for compact display. */
export function formatMs(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  if (value < 10) return value.toFixed(2) + ' ms'
  if (value < 100) return value.toFixed(1) + ' ms'
  return Math.round(value) + ' ms'
}

/** Format a 0..1 ratio as a percentage string. */
export function formatPct(ratio: number | null | undefined): string {
  if (ratio === null || ratio === undefined || Number.isNaN(ratio)) return '—'
  const digits = ratio > 0 && ratio < 0.01 ? 2 : 1
  return (ratio * 100).toFixed(digits) + '%'
}
