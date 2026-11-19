import { describe, it, expect } from 'vitest'
import { cn, formatMs, formatPct } from '../utils'

describe('lib/utils', () => {
  it('cn merges class names and resolves tailwind conflicts', () => {
    expect(cn('px-2', 'px-4')).toBe('px-4')
    expect(cn('text-sm', false && 'hidden', 'bg-card')).toBe('text-sm bg-card')
  })

  it('formatMs renders values and placeholders', () => {
    expect(formatMs(null)).toBe('—')
    expect(formatMs(undefined)).toBe('—')
    expect(formatMs(3.456)).toBe('3.46 ms')
    expect(formatMs(42.5)).toBe('42.5 ms')
    expect(formatMs(1234)).toBe('1234 ms')
  })

  it('formatPct renders ratios as percentages', () => {
    expect(formatPct(null)).toBe('—')
    expect(formatPct(0)).toBe('0.0%')
    expect(formatPct(0.005)).toBe('0.50%')
    expect(formatPct(0.123)).toBe('12.3%')
  })
})
